package llm

import "context"

// Fake is a test backend that replays canned responses in order.
type Fake struct {
	Responses []string
	Err       error
	Calls     []string // user prompts received
}

func (f *Fake) Name() string { return "fake" }

func (f *Fake) Complete(ctx context.Context, system, user string) (string, error) {
	f.Calls = append(f.Calls, user)
	if f.Err != nil {
		return "", f.Err
	}
	i := len(f.Calls) - 1
	if i >= len(f.Responses) {
		i = len(f.Responses) - 1
	}
	if i < 0 {
		return "", nil
	}
	return f.Responses[i], nil
}
