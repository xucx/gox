package moduleable

import "context"

type Module interface {
	Init(runner *Runner) error
	Run(ctx context.Context) error
}
