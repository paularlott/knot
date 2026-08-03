package api

import (
	"net/http"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/container/nomad"
	"github.com/paularlott/knot/internal/specwizard"
	"github.com/paularlott/knot/internal/util/rest"
)

// HandleGetBaseImages returns the active base image manifest (embedded or
// external per server config). The Image field's ${{ .server.base_image_registry }}
// is left as-is; resolution happens at deploy time in the spec itself.
func HandleGetBaseImages(w http.ResponseWriter, r *http.Request) {
	manifest, err := specwizard.LoadManifest(config.GetServerConfig())
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	cfg := config.GetServerConfig()
	if cfg != nil && cfg.BaseImageRegistryUser != "" && cfg.BaseImageRegistryPassword != "" {
		manifest.RegistryAuth = true
	}
	rest.WriteResponse(http.StatusOK, w, r, manifest)
}

// HandleRefreshBaseImages forces an immediate fetch of the base image manifest.
// It uses the same FetchDecision as the startup path: it fetches iff
// --base-images-update-enabled is on and (no manifest file is set, or a manifest
// file is set AND an explicit --base-images-update-url is given). The fetched
// copy overlays the baseline only when its manifest_version is newer. Exposed
// via the `knot admin refresh-base-images` CLI command.
func HandleRefreshBaseImages(w http.ResponseWriter, r *http.Request) {
	cfg := config.GetServerConfig()
	url, ok := specwizard.FetchDecision(cfg)
	if !ok {
		rest.WriteResponse(http.StatusConflict, w, r, ErrorResponse{
			Error: specwizard.FetchDisabledReason(cfg),
		})
		return
	}

	stored, err := specwizard.RefreshNow(url)
	if err != nil {
		rest.WriteResponse(http.StatusBadGateway, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	rest.WriteResponse(http.StatusOK, w, r, apiclient.BaseImageRefreshResponse{
		Updated:       stored,
		ActiveVersion: specwizard.ActiveManifestVersion(),
		UpdateURL:     url,
	})
}

// HandleGetCapabilities returns the Linux capability catalog the wizard's
// capability picker renders (name + description, searchable client-side). The
// list is a static common subset; unknown-but-well-formed CAP_* names already
// present in a spec still round-trip through the wizard.
func HandleGetCapabilities(w http.ResponseWriter, r *http.Request) {
	rest.WriteResponse(http.StatusOK, w, r, apiclient.CapabilitiesResponse{
		Capabilities: specwizard.Capabilities(),
	})
}

// HandleSpecParse converts a native template spec into the unified JSON the
// wizard edits. Returns wizardable=false if the spec is too complex for the
// wizard (multi-task Nomad job, unparseable HCL, unsupported driver, etc.);
// the UI uses that flag to disable the wizard button.
func HandleSpecParse(w http.ResponseWriter, r *http.Request) {
	request := apiclient.SpecParseRequest{}
	if err := rest.DecodeRequestBody(w, r, &request); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	spec, wizardable, reason := specwizard.Parse(request.Platform, request.Job, request.Volumes, defaultHCLParser())

	resp := apiclient.SpecParseResponse{
		Wizardable: wizardable,
		Reason:     reason,
		Spec:       spec,
	}

	// If parseable, check whether the wizard can display everything or
	// whether Advanced mode should be forced.
	if wizardable {
		fully, advReason := specwizard.CheckFullyRepresentable(request.Platform, request.Job, request.Volumes, spec)
		resp.FullyRepresentable = fully
		resp.AdvancedReason = advReason
	}

	rest.WriteResponse(http.StatusOK, w, r, resp)
}

// HandleSpecBuild converts a unified spec back into native text. On the
// Nomad side this patches the wizard's fields into the original HCL; on the
// container side it regenerates from the spec. See BuildNomadHCL /
// BuildContainerYAML for the per-platform behaviour.
func HandleSpecBuild(w http.ResponseWriter, r *http.Request) {
	request := apiclient.SpecBuildRequest{}
	if err := rest.DecodeRequestBody(w, r, &request); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	job, volumes, err := specwizard.Build(request.Platform, request.Spec, request.OriginalJob, request.OriginalVolumes)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	rest.WriteResponse(http.StatusOK, w, r, apiclient.SpecBuildResponse{
		Job:     job,
		Volumes: volumes,
	})
}

// defaultHCLParser returns a parser that delegates to a Nomad client when one
// is configured. When Nomad isn't configured (empty host), the parser is nil
// and ParseNomadHCL reports the spec as not wizardable.
func defaultHCLParser() specwizard.HCLParser {
	cfg := config.GetServerConfig()
	if cfg == nil || cfg.Nomad.Host == "" {
		return nil
	}
	return func(hcl string) (map[string]interface{}, error) {
		client, err := nomad.NewClient()
		if err != nil {
			return nil, err
		}
		return client.ParseJobHCL(hcl)
	}
}
