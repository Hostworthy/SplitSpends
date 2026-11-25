package routes

import (
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/auth"
	"github.com/Hostworthy/SplitSpends/packages/platform"
)

// TestTokensNeedSession checks that an API token can list tokens but cannot
// create or revoke them; only a signed-in session can.
func TestTokensNeedSession(t *testing.T) {
	s := newServer(t, bootstraptest.Options{})
	session := s.signIn(t, "ada@example.com", "Ada").Token
	path := platform.Prefix + "/me/tokens"
	created := data[platform.Token](t, s.call(t, http.MethodPost, path, session, platform.TokenCreate{Name: "MCP"}).want(t, http.StatusCreated))

	s.call(t, http.MethodGet, path, created.Token, nil).want(t, http.StatusOK)
	s.refused(t, created.Token, http.StatusForbidden, "FORBIDDEN", []request{
		{http.MethodPost, "/me/tokens", platform.TokenCreate{Name: "Copy"}},
		{http.MethodDelete, "/me/tokens/" + created.ID, nil},
	})
	s.call(t, http.MethodDelete, path+"/"+created.ID, session, nil).want(t, http.StatusNoContent)
}

// TestTokenLimit checks that concurrent requests never take an account past
// auth.MaxTokens API tokens.
func TestTokenLimit(t *testing.T) {
	s := newServer(t, bootstraptest.Options{})
	session := s.signIn(t, "ada@example.com", "Ada").Token
	path := platform.Prefix + "/me/tokens"

	requests := make([]*http.Request, 2*auth.MaxTokens)
	for i := range requests {
		requests[i] = s.request(t, http.MethodPost, path, platform.TokenCreate{Name: fmt.Sprintf("Token %d", i+1)})
		requests[i].Header.Set("Authorization", "Bearer "+session)
	}
	statuses := make([]int, len(requests))
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i, req := range requests {
		wg.Go(func() {
			<-start
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Error(err)
				return
			}
			statuses[i] = res.StatusCode
			_ = res.Body.Close()
		})
	}
	close(start)
	wg.Wait()

	counts := map[int]int{}
	for _, status := range statuses {
		counts[status]++
	}
	if counts[http.StatusCreated] != auth.MaxTokens || counts[http.StatusUnprocessableEntity] != auth.MaxTokens {
		t.Fatalf("want %d tokens created and %d refused, got %v", auth.MaxTokens, auth.MaxTokens, counts)
	}
	tokens := s.call(t, http.MethodGet, path, session, nil).want(t, http.StatusOK)
	if got := items[platform.Token](t, tokens); len(got.Items) != auth.MaxTokens {
		t.Fatalf("want %d stored tokens, got %d", auth.MaxTokens, len(got.Items))
	}
}
