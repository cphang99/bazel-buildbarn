package sharding

import (
	"context"
	"io"

	"github.com/EdSchouten/bazel-buildbarn/pkg/blobstore"
	"github.com/EdSchouten/bazel-buildbarn/pkg/util"
)

type shardingBlobAccess struct {
	backends           []blobstore.BlobAccess
	shardSelector      ShardSelector
	digestKeyFormat    util.DigestKeyFormat
	hashInitialization uint64
}

// NewShardingBlobAccess is an adapter for BlobAccess that partitions
// requests across backends by hashing the digest. A ShardSelector is
// used to map hashes to backends.
func NewShardingBlobAccess(backends []blobstore.BlobAccess, shardSelector ShardSelector, digestKeyFormat util.DigestKeyFormat, hashInitialization uint64) blobstore.BlobAccess {
	return &shardingBlobAccess{
		backends:           backends,
		shardSelector:      shardSelector,
		digestKeyFormat:    digestKeyFormat,
		hashInitialization: hashInitialization,
	}
}

func (ba *shardingBlobAccess) getBackend(digest *util.Digest) blobstore.BlobAccess {
	// Hash the key using FNV-1a.
	h := ba.hashInitialization
	for _, c := range digest.GetKey(ba.digestKeyFormat) {
		h ^= uint64(c)
		h *= 1099511628211
	}

	// Keep requesting shards until matching one that is undrained.
	var backend blobstore.BlobAccess
	ba.shardSelector.GetShard(h, func(index int) bool {
		backend = ba.backends[index]
		return backend == nil
	})
	return backend
}

func (ba *shardingBlobAccess) Get(ctx context.Context, digest *util.Digest) (int64, io.ReadCloser, error) {
	return ba.getBackend(digest).Get(ctx, digest)
}

func (ba *shardingBlobAccess) Put(ctx context.Context, digest *util.Digest, sizeBytes int64, r io.ReadCloser) error {
	return ba.getBackend(digest).Put(ctx, digest, sizeBytes, r)
}

func (ba *shardingBlobAccess) Delete(ctx context.Context, digest *util.Digest) error {
	return ba.getBackend(digest).Delete(ctx, digest)
}

type findMissingResults struct {
	missing []*util.Digest
	err     error
}

func callFindMissing(ctx context.Context, blobAccess blobstore.BlobAccess, digests []*util.Digest) findMissingResults {
	missing, err := blobAccess.FindMissing(ctx, digests)
	return findMissingResults{missing: missing, err: err}
}

func (ba *shardingBlobAccess) FindMissing(ctx context.Context, digests []*util.Digest) ([]*util.Digest, error) {
	// Determine which backends to contact.
	digestsPerBackend := map[blobstore.BlobAccess][]*util.Digest{}
	for _, digest := range digests {
		backend := ba.getBackend(digest)
		digestsPerBackend[backend] = append(digestsPerBackend[backend], digest)
	}

	// Asynchronously call FindMissing() on backends.
	resultsChan := make(chan findMissingResults, len(digestsPerBackend))
	for backend, digests := range digestsPerBackend {
		go func(backend blobstore.BlobAccess, digests []*util.Digest) {
			resultsChan <- callFindMissing(ctx, backend, digests)
		}(backend, digests)
	}

	// Recombine results.
	var missingDigests []*util.Digest
	var err error
	for i := 0; i < len(digestsPerBackend); i++ {
		results := <-resultsChan
		if results.err == nil {
			missingDigests = append(missingDigests, results.missing...)
		} else {
			err = results.err
		}
	}
	return missingDigests, err
}
