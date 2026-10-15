package terraforming

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSecretKeysByValue(t *testing.T) {
	for _, tc := range []struct{ config, want string }{
		{`{"connectionString":"Server=db"}`, "connectionString"},
		{`{"headers":{"Authorization":"x"}}`, "Authorization"},
		{`{"db":"Server=db;User Id=sa;Password=hunter2"}`, "db"},
		{`{"db":"server=db;uid=sa;PWD=hunter2"}`, "db"},
		{`{"upstream":"https://user:hunter2@example.com/api"}`, "upstream"},
		{`{"upstream":"redis://:hunter2@cache:6379"}`, "upstream"},
		{`{"header":"Bearer abc.def"}`, "header"},
		{`{"header":"basic dXNlcjpwYXNz"}`, "header"},
		{`{"servers":["https://user:hunter2@a.example.com"]}`, "servers"},
		{`{"servers":[{"url":"https://user:pw@b.example.com"}]}`, "url"},
	} {
		assert.Equal(t, []string{tc.want}, secretKeys(tc.config), tc.config)
	}
	for _, config := range []string{
		`{"url":"https://example.com/a@b"}`,
		`{"url":"https://user@example.com/"}`,
		`{"mail":"admin@example.com"}`,
		`{"note":"a basic setup"}`,
		`{"Auth":{"enabled":true}}`,
		`["Bearer abc"]`,
	} {
		assert.Empty(t, secretKeys(config), config)
	}
}
