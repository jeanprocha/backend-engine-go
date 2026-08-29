package reqctx

import (
	"context"
	"testing"
)

func TestWithID_FromContext_RoundTrip(t *testing.T) {
	ctx := WithID(context.Background(), "req-123")
	if got := FromContext(ctx); got != "req-123" {
		t.Errorf("FromContext() = %q, want %q", got, "req-123")
	}
}

func TestFromContext_SemValorGravado(t *testing.T) {
	if got := FromContext(context.Background()); got != "" {
		t.Errorf("FromContext() de contexto sem ID = %q, want \"\"", got)
	}
}
