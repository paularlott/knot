package nfs

import (
	"context"
)

func onRmDir(ctx context.Context, w *response, userHandle Handler) error {
	return onRemoveObj(ctx, w, userHandle, true)
}
