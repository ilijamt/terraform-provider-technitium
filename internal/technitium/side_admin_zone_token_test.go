package technitium_test

import (
	"context"
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ilijamt/terraform-provider-technitium/internal/client"
)

// The server refuses to delete the token a call is made with, as the
// provider would when destroying the token it uses.
func TestAccSideTokenDeleteOwnSession(t *testing.T) {
	acceptance(t)
	c := api(t)
	token := call(t, c, "/api/admin/sessions/createToken", url.Values{"user": {"admin"}, "tokenName": {"side-self"}})["token"].(string)
	partial := url.Values{"partialToken": {token[:16]}}
	t.Cleanup(func() {
		_, _ = c.Call(context.Background(), "/api/admin/sessions/delete", partial)
	})

	self := client.New(os.Getenv("TECHNITIUM_HOST"), token, "test", false, nil)
	_, err := self.Call(context.Background(), "/api/admin/sessions/delete", partial)
	require.ErrorContains(t, err, "cannot delete current session")

	call(t, c, "/api/admin/sessions/delete", partial)
}
