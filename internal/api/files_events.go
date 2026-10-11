package api

import (
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/filestore"
	"github.com/paularlott/knot/internal/sse"
)

// NotifyFilesChanged tells each connected user which of the buckets that
// changed they can see, or could until the change: a user hears nothing of
// buckets that aren't theirs or shared with them. An empty list, when too
// many buckets changed to name, goes to every user able to use file storage.
func NotifyFilesChanged(bucketIds []string) {
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
		if len(bucketIds) == 0 {
			sse.PublishFilesChanged(userId, nil)
			continue
		}
		var visible []string
		for _, id := range bucketIds {
			if store.MaySee(p, id) {
				visible = append(visible, id)
			}
		}
		if len(visible) > 0 {
			sse.PublishFilesChanged(userId, visible)
		}
	}
}
