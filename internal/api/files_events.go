package api

import (
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/filestore"
	"github.com/paularlott/knot/internal/sse"
)

// NotifyFilesChanged tells each connected user what changed in the buckets
// they can see, or could until the change: a user hears nothing of buckets
// that aren't theirs or shared with them. An empty list, when too much
// changed to name, goes to every user able to use file storage.
func NotifyFilesChanged(changes []filestore.BucketChanges) {
	store := filestore.Get()
	if store == nil {
		return
	}
	db := database.GetInstance()
	for _, userId := range sse.GetHub().UserIds() {
		user, err := db.GetUser(userId)
		if err != nil || user == nil {
			continue
		}
		p, ok := filestore.PrincipalFor(user)
		if !ok {
			continue
		}
		if len(changes) == 0 {
			sse.PublishFilesChanged(userId, nil, nil)
			continue
		}
		var visible []string
		var files []sse.FileChange
		for _, bc := range changes {
			if !store.MaySee(p, bc.BucketId) {
				continue
			}
			visible = append(visible, bc.BucketId)
			for _, f := range bc.Files {
				files = append(files, sse.FileChange{BucketId: bc.BucketId, Key: f.Key, Writer: f.Writer})
			}
		}
		if len(visible) > 0 {
			sse.PublishFilesChanged(userId, visible, files)
		}
	}
}
