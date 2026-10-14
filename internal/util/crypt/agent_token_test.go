package crypt

import "testing"

func TestAgentCliToken(t *testing.T) {
	const key = "0123456789abcdef0123456789abcdef"
	agent, err := GenerateAgentToken("space-1", "user-1", "zone", key)
	if err != nil {
		t.Fatal(err)
	}
	cli, err := GenerateAgentCliToken("space-1", "user-1", "zone", key)
	if err != nil {
		t.Fatal(err)
	}
	if agent == cli {
		t.Fatal("the processes' token must differ from the agent's")
	}

	for _, token := range []string{agent, cli} {
		if !IsAgentToken(token) || ExtractSpaceIdFromToken(token) != "space-1" {
			t.Errorf("%s: not recognised as a token for space-1", token)
		}
		if !ValidateAgentToken(token, "space-1", "user-1", "zone", key) {
			t.Errorf("%s: doesn't validate", token)
		}
		if ValidateAgentToken(token, "space-1", "user-2", "zone", key) || ValidateAgentToken(token, "space-2", "user-1", "zone", key) {
			t.Errorf("%s: validates for another space or user", token)
		}
	}
	if IsAgentCliToken(agent) || !IsAgentCliToken(cli) {
		t.Fatal("IsAgentCliToken must tell the two apart")
	}

	// Swapping the prefix doesn't turn one into the other.
	if ValidateAgentToken("agc_"+agent[4:], "space-1", "user-1", "zone", key) {
		t.Fatal("the agent's signature passed as the processes' token")
	}
	if ValidateAgentToken("agt_"+cli[4:], "space-1", "user-1", "zone", key) {
		t.Fatal("the processes' signature passed as the agent's token")
	}
}
