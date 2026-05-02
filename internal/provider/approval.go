package provider

import "context"

// ApprovalFunc is called when the provider encounters a permission prompt.
// It receives the cleaned prompt text and returns true to allow, false to deny.
type ApprovalFunc func(ctx context.Context, prompt string) (bool, error)

type approvalKey struct{}

// WithApproval returns a child context carrying the given approval function.
func WithApproval(ctx context.Context, fn ApprovalFunc) context.Context {
	return context.WithValue(ctx, approvalKey{}, fn)
}

// ApprovalFromContext extracts the ApprovalFunc from ctx, or nil if none set.
func ApprovalFromContext(ctx context.Context) ApprovalFunc {
	fn, _ := ctx.Value(approvalKey{}).(ApprovalFunc)
	return fn
}
