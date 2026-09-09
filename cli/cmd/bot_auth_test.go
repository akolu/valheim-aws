package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// mockSSMClient implements ssmAPI for testing.
type mockSSMClient struct {
	params map[string]string // path → value
	// Track calls
	getCalls    []string
	putCalls    []putCall
	deleteCalls []string
}

type putCall struct {
	path  string
	value string
}

func newMockSSM(initial map[string]string) *mockSSMClient {
	if initial == nil {
		initial = make(map[string]string)
	}
	return &mockSSMClient{params: initial}
}

func (m *mockSSMClient) GetParameter(ctx context.Context, params *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	name := aws.ToString(params.Name)
	m.getCalls = append(m.getCalls, name)
	val, ok := m.params[name]
	if !ok {
		return nil, &types.ParameterNotFound{Message: aws.String(fmt.Sprintf("parameter %s not found", name))}
	}
	return &ssm.GetParameterOutput{
		Parameter: &types.Parameter{
			Name:  aws.String(name),
			Value: aws.String(val),
		},
	}, nil
}

func (m *mockSSMClient) PutParameter(ctx context.Context, params *ssm.PutParameterInput, _ ...func(*ssm.Options)) (*ssm.PutParameterOutput, error) {
	name := aws.ToString(params.Name)
	val := aws.ToString(params.Value)
	m.putCalls = append(m.putCalls, putCall{path: name, value: val})
	m.params[name] = val
	return &ssm.PutParameterOutput{}, nil
}

func (m *mockSSMClient) DeleteParameter(ctx context.Context, params *ssm.DeleteParameterInput, _ ...func(*ssm.Options)) (*ssm.DeleteParameterOutput, error) {
	name := aws.ToString(params.Name)
	m.deleteCalls = append(m.deleteCalls, name)
	delete(m.params, name)
	return &ssm.DeleteParameterOutput{}, nil
}

// --- ssmGetList tests ---

func TestSsmGetList_ParameterNotFound(t *testing.T) {
	client := newMockSSM(nil)
	entries, err := ssmGetList(context.Background(), client, "/bonfire/valheim/authorized_users")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected empty list, got %v", entries)
	}
}

func TestSsmGetList_Single(t *testing.T) {
	client := newMockSSM(map[string]string{"/bonfire/valheim/authorized_users": "123456"})
	entries, err := ssmGetList(context.Background(), client, "/bonfire/valheim/authorized_users")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 || entries[0] != "123456" {
		t.Errorf("got %v, want [123456]", entries)
	}
}

func TestSsmGetList_Multiple(t *testing.T) {
	client := newMockSSM(map[string]string{"/bonfire/valheim/authorized_users": "111,222,333"})
	entries, err := ssmGetList(context.Background(), client, "/bonfire/valheim/authorized_users")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("got %v, want 3 entries", entries)
	}
}

func TestSsmGetList_TrimsSpaces(t *testing.T) {
	client := newMockSSM(map[string]string{"/p": "111, 222 , 333"})
	entries, err := ssmGetList(context.Background(), client, "/p")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, e := range entries {
		if e != "111" && e != "222" && e != "333" {
			t.Errorf("unexpected entry %q (spaces not trimmed?)", e)
		}
	}
}

func TestSsmGetList_GetError(t *testing.T) {
	client := &errorSSMClient{getErr: errors.New("access denied")}
	_, err := ssmGetList(context.Background(), client, "/p")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- ssmListAdd tests ---

func TestSsmListAdd_FirstGrant(t *testing.T) {
	client := newMockSSM(nil)
	var gotCount int
	err := ssmListAdd(context.Background(), client, "/bonfire/valheim/authorized_users", "123456", func(n int) { gotCount = n })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotCount != 1 {
		t.Errorf("count = %d, want 1", gotCount)
	}
	if client.params["/bonfire/valheim/authorized_users"] != "123456" {
		t.Errorf("param = %q, want %q", client.params["/bonfire/valheim/authorized_users"], "123456")
	}
}

func TestSsmListAdd_AddsToExisting(t *testing.T) {
	client := newMockSSM(map[string]string{"/bonfire/valheim/authorized_users": "111"})
	var gotCount int
	err := ssmListAdd(context.Background(), client, "/bonfire/valheim/authorized_users", "222", func(n int) { gotCount = n })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotCount != 2 {
		t.Errorf("count = %d, want 2", gotCount)
	}
	if client.params["/bonfire/valheim/authorized_users"] != "111,222" {
		t.Errorf("param = %q, want %q", client.params["/bonfire/valheim/authorized_users"], "111,222")
	}
}

func TestSsmListAdd_Idempotent(t *testing.T) {
	client := newMockSSM(map[string]string{"/bonfire/valheim/authorized_users": "111,222"})
	var gotCount int
	err := ssmListAdd(context.Background(), client, "/bonfire/valheim/authorized_users", "111", func(n int) { gotCount = n })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotCount != 2 {
		t.Errorf("count = %d, want 2", gotCount)
	}
	if len(client.putCalls) != 0 {
		t.Error("expected no PutParameter call for idempotent add")
	}
}

// --- ssmListRemove tests ---

func TestSsmListRemove_RemovesEntry(t *testing.T) {
	client := newMockSSM(map[string]string{"/bonfire/valheim/authorized_users": "111,222,333"})
	var gotCount int
	err := ssmListRemove(context.Background(), client, "/bonfire/valheim/authorized_users", "222", func(n int) { gotCount = n })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotCount != 2 {
		t.Errorf("count = %d, want 2", gotCount)
	}
	if client.params["/bonfire/valheim/authorized_users"] != "111,333" {
		t.Errorf("param = %q, want %q", client.params["/bonfire/valheim/authorized_users"], "111,333")
	}
}

func TestSsmListRemove_DeletesWhenEmpty(t *testing.T) {
	client := newMockSSM(map[string]string{"/bonfire/valheim/authorized_users": "111"})
	var gotCount int
	err := ssmListRemove(context.Background(), client, "/bonfire/valheim/authorized_users", "111", func(n int) { gotCount = n })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotCount != 0 {
		t.Errorf("count = %d, want 0", gotCount)
	}
	if len(client.deleteCalls) != 1 {
		t.Errorf("expected 1 DeleteParameter call, got %d", len(client.deleteCalls))
	}
	if _, ok := client.params["/bonfire/valheim/authorized_users"]; ok {
		t.Error("parameter should have been deleted")
	}
}

func TestSsmListRemove_Idempotent(t *testing.T) {
	client := newMockSSM(map[string]string{"/bonfire/valheim/authorized_users": "111,333"})
	var gotCount int
	err := ssmListRemove(context.Background(), client, "/bonfire/valheim/authorized_users", "999", func(n int) { gotCount = n })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotCount != 2 {
		t.Errorf("count = %d, want 2", gotCount)
	}
	if len(client.putCalls) != 0 {
		t.Error("expected no PutParameter call for idempotent remove")
	}
}

func TestSsmListRemove_ParameterNotFound(t *testing.T) {
	client := newMockSSM(nil)
	var gotCount int
	err := ssmListRemove(context.Background(), client, "/bonfire/valheim/authorized_users", "111", func(n int) { gotCount = n })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotCount != 0 {
		t.Errorf("count = %d, want 0", gotCount)
	}
}

// --- trust/untrust SSM path tests ---

func TestTrustPath(t *testing.T) {
	client := newMockSSM(nil)
	err := ssmListAdd(context.Background(), client, "/bonfire/allowed_guilds", "guild-123", func(n int) {})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.params["/bonfire/allowed_guilds"] != "guild-123" {
		t.Errorf("param = %q, want %q", client.params["/bonfire/allowed_guilds"], "guild-123")
	}
}

func TestUntrustPath(t *testing.T) {
	client := newMockSSM(map[string]string{"/bonfire/allowed_guilds": "guild-123,guild-456"})
	err := ssmListRemove(context.Background(), client, "/bonfire/allowed_guilds", "guild-123", func(n int) {})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.params["/bonfire/allowed_guilds"] != "guild-456" {
		t.Errorf("param = %q, want %q", client.params["/bonfire/allowed_guilds"], "guild-456")
	}
}

// errorSSMClient is an ssmAPI that always returns an error from GetParameter.
type errorSSMClient struct {
	getErr error
}

func (e *errorSSMClient) GetParameter(ctx context.Context, params *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	return nil, e.getErr
}

func (e *errorSSMClient) PutParameter(ctx context.Context, params *ssm.PutParameterInput, _ ...func(*ssm.Options)) (*ssm.PutParameterOutput, error) {
	return nil, nil
}

func (e *errorSSMClient) DeleteParameter(ctx context.Context, params *ssm.DeleteParameterInput, _ ...func(*ssm.Options)) (*ssm.DeleteParameterOutput, error) {
	return nil, nil
}

// --- botSSMClient deployment-gate tests ---

// TestBotSSMClient_DoesNotRequireTFVars pins the rule that the ACL commands
// (trust/untrust/grant/revoke) reach SSM directly and must not be gated on
// terraform/bot/terraform.tfvars. That file holds deploy-time Discord
// credentials and lives only on the operator's machine; its absence says
// nothing about whether the bot is deployed in AWS. Gating the ACL commands on
// it locks an operator out of a perfectly healthy bot on any new machine.
func TestBotSSMClient_DoesNotRequireTFVars(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, "terraform", "games"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BONFIRE_REPO_ROOT", repoRoot)

	_, err := botSSMClient(context.Background())
	if err != nil && strings.Contains(err.Error(), "Bot not deployed yet") {
		t.Errorf("ACL commands must not require terraform/bot/terraform.tfvars, got: %v", err)
	}
}

// TestRunBotUpdate_MissingTFVars is the other half of the rule: update reads
// the Discord credentials out of terraform/bot/terraform.tfvars, so it must
// still fail fast with the friendly message when that file is absent.
func TestRunBotUpdate_MissingTFVars(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, "terraform", "games"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BONFIRE_REPO_ROOT", repoRoot)

	err := runBotUpdate(nil, nil)
	if err == nil {
		t.Fatal("expected error when terraform.tfvars missing, got nil")
	}
	if !strings.Contains(err.Error(), "Bot not deployed yet") {
		t.Errorf("error should mention 'Bot not deployed yet', got: %v", err)
	}
}
