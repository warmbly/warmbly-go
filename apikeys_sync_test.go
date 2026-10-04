package warmbly

import (
	"context"
	"errors"
	"testing"
)

func TestAPIKeyDeletePermanently(t *testing.T) {
	var got syncCapture
	c := respondingClient(t, &got, 200, `{"status":"deleted"}`)
	resp, err := c.APIKeys.DeletePermanently(context.Background(), "key_1")
	if err != nil {
		t.Fatal(err)
	}
	got.wantRequest(t, "DELETE", "/v1/api-keys/key_1/permanent", "")
	got.wantBody(t, "")
	if resp.StatusCode != 200 {
		t.Errorf("status = %d", resp.StatusCode)
	}
}

func TestAPIKeyDeletePermanentlyRefusesActiveKey(t *testing.T) {
	var got syncCapture
	c := respondingClient(t, &got, 409, `{"error":"Conflict","message":"this key is still active; revoke it before deleting it","code":"conflict"}`)
	_, err := c.APIKeys.DeletePermanently(context.Background(), "key_1")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("err = %v, want a 409", err)
	}
}

func TestAPIKeyRevokeIsADifferentPath(t *testing.T) {
	var got syncCapture
	c := respondingClient(t, &got, 200, `{"status":"revoked"}`)
	if _, err := c.APIKeys.Revoke(context.Background(), "key_1", "rotated"); err != nil {
		t.Fatal(err)
	}
	got.wantRequest(t, "DELETE", "/v1/api-keys/key_1", "reason=rotated")
}
