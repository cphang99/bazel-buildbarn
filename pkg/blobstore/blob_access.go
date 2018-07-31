package blobstore

import (
	"context"
	"io"

	remoteexecution "google.golang.org/genproto/googleapis/devtools/remoteexecution/v1test"
)

type BlobAccess interface {
	Get(ctx context.Context, instance string, digest *remoteexecution.Digest) io.ReadCloser
	Put(ctx context.Context, instance string, digest *remoteexecution.Digest, r io.ReadCloser) error
	FindMissing(ctx context.Context, instance string, digests []*remoteexecution.Digest) ([]*remoteexecution.Digest, error)
}

type errorReader struct {
	err error
}

func (r *errorReader) Read(_ []byte) (int, error) {
	return 0, r.err
}

func (r *errorReader) Close() error {
	return r.err
}
