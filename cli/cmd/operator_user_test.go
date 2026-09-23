package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
)

func TestRootRegistersOperatorUserCommands(t *testing.T) {
	for _, args := range [][]string{
		{"operator", "user", "create", "--help"},
		{"operator", "user", "set-password", "--help"},
		{"operator", "user", "role", "add", "--help"},
	} {
		cmd, _, err := rootCmd.Find(args)
		if err != nil {
			t.Fatalf("find %v: %v", args, err)
		}
		if cmd == nil {
			t.Fatalf("root command did not register %v", args)
		}
	}
}

func TestOperatorUserCommandsExerciseOperatorAPI(t *testing.T) {
	env := newOperatorCLITestEnv(t)

	createOut := env.run(t, "operator", "user", "create",
		"--login", "cli-admin-user",
		"--display-name", "CLI Admin User",
		"--password-stdin",
		"--verified-email", "cli-admin@example.com",
		"--role", "cli-test-role",
		"--json",
	)
	var created struct {
		Member struct {
			User struct {
				Login string `json:"login"`
			} `json:"user"`
			Roles []string `json:"roles"`
		} `json:"member"`
	}
	if err := json.Unmarshal([]byte(createOut), &created); err != nil {
		t.Fatalf("unmarshal create output: %v\n%s", err, createOut)
	}
	if created.Member.User.Login != "cli-admin-user" || strings.Join(created.Member.Roles, ",") != "cli-test-role" {
		t.Fatalf("create output = %+v", created.Member)
	}
	user, err := env.core.GetUserByLogin(env.ctx, "cli-admin-user")
	if err != nil {
		t.Fatalf("GetUserByLogin after create: %v", err)
	}
	emails, err := env.core.GetVerifiedEmails(env.ctx, user.Id)
	if err != nil {
		t.Fatalf("GetVerifiedEmails: %v", err)
	}
	if len(emails) != 1 || emails[0].Email != "cli-admin@example.com" {
		t.Fatalf("verified emails = %+v, want cli-admin@example.com", emails)
	}
	roles, err := env.core.GetUserRoles(env.ctx, user.Id)
	if err != nil {
		t.Fatalf("GetUserRoles: %v", err)
	}
	if strings.Join(roles, ",") != "cli-test-role" {
		t.Fatalf("roles = %v, want cli-test-role", roles)
	}

	getOut := env.run(t, "operator", "user", "get", user.Id)
	if !strings.Contains(getOut, user.Id+"\tcli-admin-user\tCLI Admin User") {
		t.Fatalf("get output = %q", getOut)
	}
	getByLoginOut := env.run(t, "operator", "user", "get", "--login", "cli-admin-user")
	if getByLoginOut != getOut {
		t.Fatalf("get by login output = %q, want %q", getByLoginOut, getOut)
	}
	getByLoginJSON := env.run(t, "operator", "user", "get", "--login", "cli-admin-user", "--json")
	var lookedUp struct {
		Member struct {
			User struct {
				ID string `json:"id"`
			} `json:"user"`
		} `json:"member"`
	}
	if err := json.Unmarshal([]byte(getByLoginJSON), &lookedUp); err != nil {
		t.Fatalf("unmarshal login lookup output: %v\n%s", err, getByLoginJSON)
	}
	if lookedUp.Member.User.ID != user.Id {
		t.Fatalf("get by login JSON user ID = %q, want %q", lookedUp.Member.User.ID, user.Id)
	}
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "missing selector", args: []string{"operator", "user", "get"}, want: "provide USER_ID or a non-empty --login"},
		{name: "empty login", args: []string{"operator", "user", "get", "--login", ""}, want: "provide USER_ID or a non-empty --login"},
		{name: "conflicting selectors", args: []string{"operator", "user", "get", user.Id, "--login", "cli-admin-user"}, want: "provide USER_ID or --login, not both"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := env.execute(t, tc.args...)
			if err == nil || err.Error() != tc.want {
				t.Fatalf("get error = %v, want %q", err, tc.want)
			}
		})
	}
	_, err = env.execute(t, "operator", "user", "get", "--login", "unknown-cli-user")
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("unknown login error = %v, want not found", err)
	}

	updateOut := env.run(t, "operator", "user", "update", user.Id, "--display-name", "CLI Renamed")
	if !strings.Contains(updateOut, "\tCLI Renamed\t") {
		t.Fatalf("update output = %q", updateOut)
	}

	passwordPath := t.TempDir() + "/password"
	if err := os.WriteFile(passwordPath, []byte("new-password-123\n"), 0o600); err != nil {
		t.Fatalf("write password file: %v", err)
	}
	env.run(t, "operator", "user", "set-password", user.Id, "--password-file", passwordPath)
	if _, _, err := env.core.VerifyPasswordWithAuthGeneration(env.ctx, "cli-admin-user", "new-password-123"); err != nil {
		t.Fatalf("VerifyPasswordWithAuthGeneration after set-password: %v", err)
	}

	emailOut := env.run(t, "operator", "user", "add-email", user.Id, "--email", "cli-admin-2@example.com")
	if !strings.Contains(emailOut, "cli-admin-2@example.com") {
		t.Fatalf("add-email output = %q", emailOut)
	}

	roleAddOut := env.run(t, "operator", "user", "role", "add", user.Id, "cli-extra-role")
	if !strings.Contains(roleAddOut, "cli-extra-role") {
		t.Fatalf("role add output = %q", roleAddOut)
	}
	roleRemoveOut := env.run(t, "operator", "user", "role", "remove", user.Id, "cli-extra-role")
	if strings.Contains(roleRemoveOut, "cli-extra-role") {
		t.Fatalf("role remove output still contains cli-extra-role: %q", roleRemoveOut)
	}

	listOut := env.run(t, "operator", "user", "list", "--search", "cli-admin", "--limit", "101")
	if !strings.Contains(listOut, "total=1 has_more=false") || !strings.Contains(listOut, "cli-admin-user") {
		t.Fatalf("list output = %q", listOut)
	}
	negativeLimitListOut := env.run(t, "operator", "user", "list", "--search", "cli-admin", "--limit", "-1")
	if !strings.Contains(negativeLimitListOut, "total=1 has_more=false") || !strings.Contains(negativeLimitListOut, "cli-admin-user") {
		t.Fatalf("list with negative limit output = %q", negativeLimitListOut)
	}
	emailListOut := env.run(t, "operator", "user", "list", "--search", "cli-admin-2@example.com")
	if !strings.Contains(emailListOut, "total=1 has_more=false") || !strings.Contains(emailListOut, user.Id) {
		t.Fatalf("list with email search output = %q", emailListOut)
	}

	deleteOut := env.run(t, "operator", "user", "delete", user.Id, "--yes")
	if !strings.Contains(deleteOut, "deleted user "+user.Id) {
		t.Fatalf("delete output = %q", deleteOut)
	}
	if _, err := env.core.GetUser(env.ctx, user.Id); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("GetUser after delete err = %v, want ErrNotFound", err)
	}
}

func TestOperatorSecretReaders(t *testing.T) {
	path := t.TempDir() + "/secret"
	if err := os.WriteFile(path, []byte("secret value\n"), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	if got, err := readSecretFile(path); err != nil || got != "secret value" {
		t.Fatalf("readSecretFile() = %q, %v; want secret value, nil", got, err)
	}

	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	t.Cleanup(func() { os.Stdin = oldStdin })
	os.Stdin = r
	if _, err := w.WriteString("stdin secret\n"); err != nil {
		t.Fatalf("write stdin pipe: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close stdin writer: %v", err)
	}
	if got, err := readSecretStdin(); err != nil || got != "stdin secret" {
		t.Fatalf("readSecretStdin() = %q, %v; want stdin secret, nil", got, err)
	}
}
