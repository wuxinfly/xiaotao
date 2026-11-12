package modelsource

import "testing"

func TestProtocolPersistenceAndURLs(t *testing.T) {
	for protocol, path := range map[string]string{"openai-chat": "/chat/completions", "openai-responses": "/responses", "anthropic-messages": "/messages"} {
		t.Run(protocol, func(t *testing.T) {
			enc, dec := testCrypto(t)
			dir := t.TempDir()
			s, _ := Open(dir, enc, dec)
			in := input()
			in.Protocol = protocol
			in.BaseURL = "https://example.com/custom/v1" + path
			src, err := s.Save("", in)
			if err != nil {
				t.Fatal(err)
			}
			if src.BaseURL != "https://example.com/custom/v1" || ProtocolURL(src.BaseURL, protocol) != in.BaseURL {
				t.Fatal(src)
			}
			s, err = Open(dir, enc, dec)
			if err != nil || s.List()[0].Protocol != protocol {
				t.Fatal(err)
			}
			in.Protocol = "openai-chat"
			in.BaseURL = "https://example.com/api"
			in.Models = src.Models
			in.APIKey = nil
			edited, err := s.Save(src.ID, in)
			if err != nil || edited.Models[0].ID != src.Models[0].ID {
				t.Fatal(edited, err)
			}
			if ProtocolURL(edited.BaseURL, edited.Protocol) != "https://example.com/api/chat/completions" {
				t.Fatal("guessed v1")
			}
		})
	}
	s := openTest(t)
	in := input()
	in.Protocol = "unsupported"
	if _, err := s.Save("", in); err == nil {
		t.Fatal("unsupported protocol accepted")
	}
}
