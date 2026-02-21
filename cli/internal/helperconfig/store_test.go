package helperconfig

import (
	"path/filepath"
	"strings"
	"testing"

	"shellman/cli/internal/projectstate"
)

func TestStore_SaveAndLoad_OpenAIConfig_EncryptsAPIKey(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "shellman.db")
	secretPath := filepath.Join(t.TempDir(), ".secret")
	if err := projectstate.InitGlobalDB(dbPath); err != nil {
		t.Fatal(err)
	}
	db, err := projectstate.GlobalDBGORM()
	if err != nil {
		t.Fatal(err)
	}
	st, err := NewStore(db, secretPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	in := OpenAIConfig{Endpoint: "https://api.openai.com", Model: "gpt-5", APIKey: "sk-test-123"}
	if err := st.SaveOpenAI(in); err != nil {
		t.Fatal(err)
	}

	got, err := st.LoadOpenAI()
	if err != nil {
		t.Fatal(err)
	}
	if got.APIKey != in.APIKey {
		t.Fatalf("want decrypted api key")
	}
	if !got.APIKeySet {
		t.Fatalf("expected api key set flag true")
	}

	raw, err := st.rawValue("helper_openai_api_key_enc")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "sk-test-123") {
		t.Fatalf("api key stored in plaintext")
	}

	if err := st.SaveOpenAI(OpenAIConfig{
		Endpoint: "https://example.com",
		Model:    "gpt-5-mini",
	}); err != nil {
		t.Fatal(err)
	}
	got2, err := st.LoadOpenAI()
	if err != nil {
		t.Fatal(err)
	}
	if got2.APIKey != "sk-test-123" || !got2.APIKeySet {
		t.Fatalf("empty api key update should not overwrite existing encrypted key: %+v", got2)
	}
}
