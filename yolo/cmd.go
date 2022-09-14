package yolo

import (
	"context"
	"io"
)

type Command interface {
	io.Closer
	Run(ctx context.Context) error
}
