package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"os/exec"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"

	"github.com/paularlott/knot/internal/container/kvm"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/util"
	"github.com/paularlott/knot/internal/util/validate"
)

// HandleSpacesConsoleProxy bridges a websocket to a KVM space's serial
// console (`virsh console` under a pty), giving the browser access to the VM
// itself: login works with the owner's knot username and service password
// even when the agent never connected. Only the node that owns the VM can
// serve it — virsh talks to the local libvirt.
func HandleSpacesConsoleProxy(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	spaceId := r.PathValue("space_id")
	if !validate.UUID(spaceId) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Check user access to the space
	if space.UserId != user.Id && !space.IsSharedWith(user.Id) {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	if !user.HasPermission(model.PermissionUseWebTerminal) {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	template, err := db.GetTemplate(space.TemplateId)
	if err != nil || template == nil || !template.IsKvm() {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	if space.ContainerId == "" {
		w.WriteHeader(http.StatusConflict)
		return
	}

	// A space on another node has its VM — and therefore its console — on
	// that node's libvirt; this server cannot reach it.
	if forward, _ := service.ShouldForwardToNode(space.NodeId); forward {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	running, err := kvm.NewClient().DomainRunning(r.Context(), space.ContainerId)
	if err != nil {
		log.WithError(err).Error("console: checking domain state", "domain", space.ContainerId)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !running {
		w.WriteHeader(http.StatusConflict)
		return
	}

	conn := util.UpgradeToWS(w, r)
	if conn == nil {
		log.Error("console: error while upgrading to websocket")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// virsh console needs a tty on stdin; the pty is also what carries the
	// terminal size. Killing the process ends both directions.
	cmd := exec.Command("virsh", "--connect", "qemu:///system", "console", space.ContainerId)
	ptmx, err := pty.Start(cmd)
	if err != nil {
		log.WithError(err).Error("console: failed to start virsh console", "domain", space.ContainerId)
		conn.Close()
		return
	}

	consoleDone := make(chan struct{})

	// pty -> websocket. Closing the websocket on exit unblocks the reader
	// below and tells the browser the console ended.
	go func() {
		defer close(consoleDone)
		defer conn.Close()
		buffer := make([]byte, 2048)
		for {
			n, err := ptmx.Read(buffer)
			if n > 0 {
				if werr := conn.WriteMessage(websocket.BinaryMessage, buffer[:n]); werr != nil {
					break
				}
			}
			if err != nil {
				break
			}
		}
		_ = cmd.Process.Kill()
	}()

	// websocket -> pty. The terminal client speaks the same wire protocol as
	// the agent terminal: keystrokes as text (or raw binary) frames, and
	// resize messages as a binary frame led by \x01 and a JSON size.
	go func() {
		for {
			mt, reader, err := conn.NextReader()
			if err != nil {
				break
			}

			data, err := io.ReadAll(reader)
			if err != nil {
				break
			}

			if mt == websocket.BinaryMessage && len(data) > 0 && data[0] == 1 {
				var size struct {
					Cols uint16 `json:"cols"`
					Rows uint16 `json:"rows"`
				}
				if err := json.Unmarshal(data[1:], &size); err == nil && size.Cols > 0 && size.Rows > 0 {
					_ = pty.Setsize(ptmx, &pty.Winsize{Rows: size.Rows, Cols: size.Cols})
				}
				continue
			}

			if _, err := ptmx.Write(data); err != nil {
				break
			}
		}
		_ = cmd.Process.Kill()
		_ = ptmx.Close()
	}()

	<-consoleDone
}
