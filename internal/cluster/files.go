package cluster

import (
	"context"
	"errors"
	"io"
	"math/rand"
	"sync"
	"time"

	"github.com/paularlott/gossip"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/filestore"
)

// File storage replication: bucket and object records travel by gossip and
// are reconciled by periodic anti-entropy; content is streamed directly from
// a peer, never gossiped, to any server that does not hold it.
//
// Anti-entropy compares bucket digests, paged; for a bucket that differs it
// compares the bucket's digest slots and exchanges only the records of the
// slots that differ, so a small difference in a large bucket moves a small
// amount of data.
//
// Leaf nodes keep their files to themselves. Leaves talk to their origin over
// the leaf protocol, which carries no file messages, and as a second line of
// defence each server advertises its files domain — origin or leaf — and only
// replicates with, or accepts file records and content requests from, servers
// in the same domain. A leaf that somehow shares a gossip cluster with origin
// servers therefore never exchanges files with them.

const (
	filesMetadataKey    = "files"
	filesDomainOrigin   = "1" // the value servers have always advertised
	filesDomainLeaf     = "leaf"
	filesSyncInterval   = 30 * time.Second
	filesPageSize       = 500 // object records per page
	filesDigestPageSize = 100 // bucket digests per page; a bucket can carry many grants
)

type filesUpdate struct {
	Buckets []*filestore.Bucket `msgpack:"buckets"`
	Objects []*filestore.Object `msgpack:"objects"`
}

type filesDigestRequest struct {
	After string `msgpack:"after"`
}

type filesDigestPage struct {
	Buckets map[string]filestore.BucketDigest `msgpack:"buckets"`
	Next    string                            `msgpack:"next"`
}

type filesSlotsRequest struct {
	Bucket string `msgpack:"bucket"`
}

type filesSlots struct {
	Digests []uint64 `msgpack:"digests"`
	Counts  []int32  `msgpack:"counts"`
}

type filesPageRequest struct {
	Bucket string `msgpack:"bucket"`
	After  string `msgpack:"after"`
	Slots  []int  `msgpack:"slots"` // only these digest slots; all when empty
}

// filesPage carries records either way: a page asked for, or records pushed.
type filesPage struct {
	Buckets []*filestore.Bucket `msgpack:"buckets"`
	Objects []*filestore.Object `msgpack:"objects"`
	Next    string              `msgpack:"next"`
}

type filesContentRequest struct {
	SHA256 string `msgpack:"sha256"`
	Offset int64  `msgpack:"offset"`
}

type filesContentBatchRequest struct {
	SHA256s []string `msgpack:"sha256s"`
}

type filesAck struct{}

// initFiles connects the file store to the cluster.
func (c *Cluster) initFiles() {
	store := filestore.Get()
	if store == nil || c.gossipCluster == nil {
		return
	}

	store.SetNodeId(c.gossipCluster.LocalNode().ID.String())
	c.gossipCluster.LocalMetadata().SetString(filesMetadataKey, localFilesDomain())

	c.gossipCluster.HandleFunc(FilesUpdateMsg, c.handleFilesUpdate)
	c.gossipCluster.HandleFuncWithReply(FilesDigestMsg, c.handleFilesDigest)
	c.gossipCluster.HandleFuncWithReply(FilesSlotsMsg, c.handleFilesSlots)
	c.gossipCluster.HandleFuncWithReply(FilesPageMsg, c.handleFilesPage)
	c.gossipCluster.HandleFuncWithReply(FilesPushMsg, c.handleFilesPush)
	c.gossipCluster.HandleStreamFunc(FilesContentMsg, c.handleFilesContent)
	c.gossipCluster.HandleStreamFunc(FilesContentBatchMsg, c.handleFilesContentBatch)

	// A server that comes back reconciles straight away.
	c.gossipCluster.HandleNodeStateChangeFunc(func(node *gossip.Node, prev gossip.NodeState) {
		if node.Alive() && prev != gossip.NodeAlive && c.isFilesNode(node) {
			go c.syncFilesWith(node)
		}
	})

	store.SetReplicator(c)
}

// filesSyncStop ends the periodic anti-entropy when the cluster stops.
var (
	filesSyncStop     = make(chan struct{})
	filesSyncStopOnce sync.Once
)

// startFilesSync runs periodic anti-entropy with a random peer until the
// cluster stops.
func (c *Cluster) startFilesSync() {
	if filestore.Get() == nil || c.gossipCluster == nil {
		return
	}

	go func() {
		// Catch up with every peer on start.
		for _, node := range c.filesNodes() {
			select {
			case <-filesSyncStop:
				return
			default:
			}
			c.syncFilesWith(node)
		}

		for {
			timer := time.NewTimer(filesSyncInterval + time.Duration(rand.Int63n(int64(filesSyncInterval/2))))
			select {
			case <-filesSyncStop:
				timer.Stop()
				return
			case <-timer.C:
			}
			nodes := c.filesNodes()
			if len(nodes) > 0 {
				c.syncFilesWith(nodes[rand.Intn(len(nodes))])
			}
		}
	}()
}

// stopFilesSync ends the periodic anti-entropy.
func (c *Cluster) stopFilesSync() {
	filesSyncStopOnce.Do(func() { close(filesSyncStop) })
}

// localFilesDomain is the files domain this server replicates within.
func localFilesDomain() string {
	if cfg := config.GetServerConfig(); cfg != nil && cfg.LeafNode {
		return filesDomainLeaf
	}
	return filesDomainOrigin
}

// isFilesNode reports whether node stores files in this server's domain.
func (c *Cluster) isFilesNode(node *gossip.Node) bool {
	return node != nil && node.Metadata != nil && node.Metadata.GetString(filesMetadataKey) == localFilesDomain()
}

func (c *Cluster) filesNodes() []*gossip.Node {
	local := c.gossipCluster.LocalNode().ID
	var nodes []*gossip.Node
	for _, n := range c.gossipCluster.AliveNodes() {
		if n.ID != local && c.isFilesNode(n) {
			nodes = append(nodes, n)
		}
	}
	return nodes
}

// The Replicator methods below are only installed with a gossip cluster.

// BroadcastFiles implements filestore.Replicator.
func (c *Cluster) BroadcastFiles(buckets []*filestore.Bucket, objects []*filestore.Object) {
	update := filesUpdate{Buckets: buckets, Objects: objects}
	if err := c.gossipCluster.Send(FilesUpdateMsg, &update); err != nil {
		c.logger.WithError(err).Debug("failed to gossip file update")
	}
}

// FileNodes implements filestore.Replicator.
func (c *Cluster) FileNodes() []string {
	nodes := c.filesNodes()
	ids := make([]string, len(nodes))
	for i, n := range nodes {
		ids[i] = n.ID.String()
	}
	return ids
}

// OpenContent implements filestore.Replicator: the content is streamed
// directly from the node.
func (c *Cluster) OpenContent(ctx context.Context, nodeId, sha string, offset int64) (io.ReadCloser, error) {
	node := c.gossipCluster.GetNodeByIDString(nodeId)
	if node == nil || !node.Alive() {
		return nil, errors.New("node not available")
	}
	return c.gossipCluster.OpenStream(ctx, node, FilesContentMsg, &filesContentRequest{SHA256: sha, Offset: offset})
}

// OpenContentBatch implements filestore.Replicator: several small blobs are
// streamed over one connection.
func (c *Cluster) OpenContentBatch(ctx context.Context, nodeId string, shas []string) (io.ReadCloser, error) {
	node := c.gossipCluster.GetNodeByIDString(nodeId)
	if node == nil || !node.Alive() {
		return nil, errors.New("node not available")
	}
	return c.gossipCluster.OpenStream(ctx, node, FilesContentBatchMsg, &filesContentBatchRequest{SHA256s: shas})
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// filesRequest decodes a file message, returning the store, or nil when
// this server stores no files or the sender is outside its files domain.
func filesRequest[T any](c *Cluster, sender *gossip.Node, packet *gossip.Packet, req *T) (*filestore.Store, error) {
	store := filestore.Get()
	if store == nil || !c.isFilesNode(sender) {
		return nil, nil
	}
	if err := packet.Unmarshal(req); err != nil {
		return nil, err
	}
	return store, nil
}

func (c *Cluster) handleFilesUpdate(sender *gossip.Node, packet *gossip.Packet) error {
	var update filesUpdate
	store, err := filesRequest(c, sender, packet, &update)
	if store != nil {
		store.Merge(update.Buckets, update.Objects)
	}
	return err
}

func (c *Cluster) handleFilesDigest(sender *gossip.Node, packet *gossip.Packet) (interface{}, error) {
	var req filesDigestRequest
	store, err := filesRequest(c, sender, packet, &req)
	if store == nil {
		return &filesDigestPage{}, err
	}
	buckets, next := store.Digests(req.After, filesDigestPageSize)
	return &filesDigestPage{Buckets: buckets, Next: next}, nil
}

func (c *Cluster) handleFilesSlots(sender *gossip.Node, packet *gossip.Packet) (interface{}, error) {
	var req filesSlotsRequest
	store, err := filesRequest(c, sender, packet, &req)
	if store == nil {
		return &filesSlots{}, err
	}
	digests, counts := store.SlotDigests(req.Bucket)
	return &filesSlots{Digests: digests, Counts: counts}, nil
}

func (c *Cluster) handleFilesPage(sender *gossip.Node, packet *gossip.Packet) (interface{}, error) {
	var req filesPageRequest
	store, err := filesRequest(c, sender, packet, &req)
	if store == nil {
		return &filesPage{}, err
	}
	objects, next := store.ObjectPage(req.Bucket, req.After, filesPageSize, slotsOrAll(req.Slots))
	return &filesPage{Objects: objects, Next: next}, nil
}

func (c *Cluster) handleFilesPush(sender *gossip.Node, packet *gossip.Packet) (interface{}, error) {
	var page filesPage
	store, err := filesRequest(c, sender, packet, &page)
	if store != nil {
		store.Merge(page.Buckets, page.Objects)
	}
	return &filesAck{}, err
}

func (c *Cluster) handleFilesContent(sender *gossip.Node, packet *gossip.Packet, w io.Writer) error {
	var req filesContentRequest
	store, err := filesRequest(c, sender, packet, &req)
	if err != nil {
		return err
	}
	if store == nil {
		return filestore.ErrUnavailable
	}
	return store.WriteContent(w, req.SHA256, req.Offset)
}

// maxContentBatch bounds how many blobs one batch request may ask for.
const maxContentBatch = 256

func (c *Cluster) handleFilesContentBatch(sender *gossip.Node, packet *gossip.Packet, w io.Writer) error {
	var req filesContentBatchRequest
	store, err := filesRequest(c, sender, packet, &req)
	if err != nil {
		return err
	}
	if store == nil {
		return filestore.ErrUnavailable
	}
	if len(req.SHA256s) > maxContentBatch {
		return filestore.ErrContentMismatch
	}
	return store.WriteContentBatch(w, req.SHA256s)
}

// slotsOrAll maps an empty slot list, which means every slot, to nil.
func slotsOrAll(slots []int) []int {
	if len(slots) == 0 {
		return nil
	}
	return slots
}

// ---------------------------------------------------------------------------
// Anti-entropy
// ---------------------------------------------------------------------------

// filesSyncing holds the peers a reconcile is running with, so a node
// coming back and the periodic round never reconcile with it at once.
var filesSyncing sync.Map

func (c *Cluster) syncFilesWith(node *gossip.Node) {
	store := filestore.Get()
	if store == nil {
		return
	}
	if _, busy := filesSyncing.LoadOrStore(node.ID, struct{}{}); busy {
		return
	}
	defer filesSyncing.Delete(node.ID)

	// The peer's bucket digests, a page at a time.
	remote := make(map[string]filestore.BucketDigest)
	for after := ""; ; {
		var page filesDigestPage
		if err := c.gossipCluster.SendToWithResponse(node, FilesDigestMsg, &filesDigestRequest{After: after}, &page); err != nil {
			c.logger.WithError(err).Debug("file digest exchange failed", "node", node.ID.String())
			return
		}
		for name, d := range page.Buckets {
			remote[name] = d
		}
		if page.Next == "" {
			break
		}
		after = page.Next
	}

	// Bucket records first, both ways, so object records land against the
	// right generation.
	var records []*filestore.Bucket
	for _, d := range remote {
		if d.Bucket != nil {
			records = append(records, d.Bucket)
		}
	}
	store.Merge(records, nil)
	newer := store.NewerBuckets(remote)
	for len(newer) > 0 {
		n := min(len(newer), filesDigestPageSize)
		if err := c.gossipCluster.SendToWithResponse(node, FilesPushMsg, &filesPage{Buckets: newer[:n]}, &filesAck{}); err != nil {
			c.logger.WithError(err).Debug("file bucket push failed", "node", node.ID.String())
			return
		}
		newer = newer[n:]
	}

	// Then the records of each bucket that differs, slot by slot.
	var buckets, pulled, pushed int
	local, _ := store.Digests("", 0)
	for name, l := range local {
		r, known := remote[name]
		if known && r.Digest == l.Digest && r.Count == l.Count {
			continue
		}
		if !known && l.Count == 0 {
			continue
		}
		in, out, err := c.syncFilesBucket(store, node, name, known)
		pulled += in
		pushed += out
		if err != nil {
			c.logger.WithError(err).Debug("file bucket sync failed", "bucket", name, "node", node.ID.String())
			continue
		}
		buckets++
	}

	if buckets > 0 {
		c.logger.Debug("file records reconciled", "node", node.ID.String(), "buckets", len(local),
			"synced_buckets", buckets, "pulled_records", pulled, "pushed_records", pushed)
	}
}

// syncFilesBucket reconciles one bucket's records with node, exchanging only
// the digest slots that differ, and returns the records pulled and pushed.
func (c *Cluster) syncFilesBucket(store *filestore.Store, node *gossip.Node, bucket string, known bool) (int, int, error) {
	if !known {
		// The peer has no records for it: send them all.
		n, err := c.pushFilesSlots(store, node, bucket, nil)
		return 0, n, err
	}

	var rs filesSlots
	if err := c.gossipCluster.SendToWithResponse(node, FilesSlotsMsg, &filesSlotsRequest{Bucket: bucket}, &rs); err != nil {
		return 0, 0, err
	}
	if len(rs.Digests) != filestore.DigestSlots || len(rs.Counts) != filestore.DigestSlots {
		return 0, 0, errors.New("malformed slot digests")
	}
	differ := func() []int {
		ld, lc := store.SlotDigests(bucket)
		var slots []int
		for i := range ld {
			if ld[i] != rs.Digests[i] || lc[i] != rs.Counts[i] {
				slots = append(slots, i)
			}
		}
		return slots
	}

	slots := differ()
	if len(slots) == 0 {
		return 0, 0, nil
	}
	pulled, err := c.pullFilesSlots(store, node, bucket, slots)
	if err != nil {
		return pulled, 0, err
	}
	// Whatever still differs from what the peer had, the peer lacks.
	pushed := 0
	if slots = differ(); len(slots) > 0 {
		pushed, err = c.pushFilesSlots(store, node, bucket, slots)
	}
	return pulled, pushed, err
}

// pullFilesSlots pages a bucket's records in the given slots from node.
func (c *Cluster) pullFilesSlots(store *filestore.Store, node *gossip.Node, bucket string, slots []int) (int, error) {
	total := 0
	for after := ""; ; {
		var page filesPage
		req := filesPageRequest{Bucket: bucket, After: after, Slots: slots}
		if err := c.gossipCluster.SendToWithResponse(node, FilesPageMsg, &req, &page); err != nil {
			return total, err
		}
		store.Merge(nil, page.Objects)
		total += len(page.Objects)
		if page.Next == "" {
			return total, nil
		}
		after = page.Next
	}
}

// pushFilesSlots pages a bucket's records in the given slots (all when nil)
// to node.
func (c *Cluster) pushFilesSlots(store *filestore.Store, node *gossip.Node, bucket string, slots []int) (int, error) {
	total := 0
	for after := ""; ; {
		objects, next := store.ObjectPage(bucket, after, filesPageSize, slots)
		if len(objects) > 0 {
			if err := c.gossipCluster.SendToWithResponse(node, FilesPushMsg, &filesPage{Objects: objects}, &filesAck{}); err != nil {
				return total, err
			}
			total += len(objects)
		}
		if next == "" {
			return total, nil
		}
		after = next
	}
}
