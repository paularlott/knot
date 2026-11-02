package apiclient

import (
	"context"
	"time"
)

type PasskeyInfo struct {
	Id             string     `json:"passkey_id" msgpack:"passkey_id"`
	Name           string     `json:"name" msgpack:"name"`
	BackupEligible bool       `json:"backup_eligible" msgpack:"backup_eligible"`
	BackupState    bool       `json:"backup_state" msgpack:"backup_state"`
	CreatedAt      time.Time  `json:"created_at" msgpack:"created_at"`
	LastUsedAt     *time.Time `json:"last_used_at" msgpack:"last_used_at"`
}

type PasskeyList struct {
	Count    int           `json:"count" msgpack:"count"`
	Passkeys []PasskeyInfo `json:"passkeys" msgpack:"passkeys"`
}

func (c *ApiClient) GetUserPasskeys(ctx context.Context, userId string) (*PasskeyList, int, error) {
	response := &PasskeyList{}
	code, err := c.httpClient.Get(ctx, "/api/users/"+userId+"/passkeys", response)
	if err != nil {
		return nil, code, err
	}
	return response, code, nil
}

func (c *ApiClient) DeleteUserPasskey(ctx context.Context, userId string, passkeyId string) (int, error) {
	return c.httpClient.Delete(ctx, "/api/users/"+userId+"/passkeys/"+passkeyId, nil, nil, 200)
}
