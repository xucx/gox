package moduleable

import (
	"context"
	"fmt"

	"github.com/ash2k/stager"
)

type Runner struct {
	modules []Module
}

func NewRunner() *Runner {
	return &Runner{}
}

func (r *Runner) Run(ctx context.Context, modules ...Module) error {
	r.modules = append([]Module{}, modules...)

	for _, module := range r.modules {
		if err := module.Init(r); err != nil {
			return err
		}
	}

	stager := stager.New()
	for _, module := range r.modules {
		stager.NextStage().Go(func(ctx context.Context) error {
			if err := module.Run(ctx); err != nil {
				return err
			}
			return nil
		})
	}

	return stager.Run(ctx)
}

func GetModule[T any](r *Runner) (T, error) {

	for _, model := range r.modules {
		if m, ok := model.(T); ok {
			return m, nil
		}
	}
	var m T
	return m, fmt.Errorf("module not found")
}

func RunWithModule[T any](r *Runner, f func(m T) error) error {

	for _, model := range r.modules {
		if m, ok := model.(T); ok {
			return f(m)
		}
	}
	return fmt.Errorf("module not found")
}
