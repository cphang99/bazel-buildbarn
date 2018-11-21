package blobstore

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/EdSchouten/bazel-buildbarn/pkg/util"
	remoteexecution "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/google/uuid"

	"google.golang.org/genproto/googleapis/bytestream"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type contentAddressableStorageBlobAccess struct {
	byteStreamClient                bytestream.ByteStreamClient
	contentAddressableStorageClient remoteexecution.ContentAddressableStorageClient
	readChunkSize                   int
}

// NewContentAddressableStorageBlobAccess creates a BlobAccess handle
// that relays any requests to a GRPC service that implements the
// bytestream.ByteStream and remoteexecution.ContentAddressableStorage
// services. Those are the services that Bazel uses to access blobs
// stored in the Content Addressable Storage.
func NewContentAddressableStorageBlobAccess(client *grpc.ClientConn, readChunkSize int) BlobAccess {
	return &contentAddressableStorageBlobAccess{
		byteStreamClient:                bytestream.NewByteStreamClient(client),
		contentAddressableStorageClient: remoteexecution.NewContentAddressableStorageClient(client),
		readChunkSize:                   readChunkSize,
	}
}

type byteStreamBlobReader struct {
	client  bytestream.ByteStream_ReadClient
	partial []byte
}

func (r *byteStreamBlobReader) Read(p []byte) (int, error) {
	// Chunk of data left from previous call.
	if len(r.partial) > 0 {
		n := copy(p, r.partial)
		r.partial = r.partial[n:]
		return n, nil
	}

	// Read next chunk.
	chunk, err := r.client.Recv()
	if err != nil {
		return 0, err
	}
	n := copy(p, chunk.Data)
	r.partial = chunk.Data[n:]
	return n, nil
}

func (r *byteStreamBlobReader) Close() error {
	return nil
}

func (ba *contentAddressableStorageBlobAccess) Get(ctx context.Context, digest *util.Digest) (int64, io.ReadCloser, error) {
	var readRequest bytestream.ReadRequest
	sizeBytes := digest.GetSizeBytes()
	if instance := digest.GetInstance(); instance == "" {
		readRequest.ResourceName = fmt.Sprintf("blobs/%s/%d", hex.EncodeToString(digest.GetHash()), sizeBytes)
	} else {
		readRequest.ResourceName = fmt.Sprintf("%s/blobs/%s/%d", instance, hex.EncodeToString(digest.GetHash()), sizeBytes)
	}
	client, err := ba.byteStreamClient.Read(ctx, &readRequest)
	if err != nil {
		return 0, nil, err
	}

	// Read first chunk to detect errors eagerly.
	chunk, err := client.Recv()
	if err != nil && err != io.EOF {
		return 0, nil, err
	}
	return sizeBytes, &byteStreamBlobReader{
		client:  client,
		partial: chunk.Data,
	}, nil
}

func (ba *contentAddressableStorageBlobAccess) Put(ctx context.Context, digest *util.Digest, sizeBytes int64, r io.ReadCloser) error {
	defer r.Close()

	client, err := ba.byteStreamClient.Write(ctx)
	if err != nil {
		return err
	}

	var resourceName string
	if instance := digest.GetInstance(); instance == "" {
		resourceName = fmt.Sprintf("uploads/%s/blobs/%s/%d", uuid.Must(uuid.NewRandom()), hex.EncodeToString(digest.GetHash()), digest.GetSizeBytes())
	} else {
		resourceName = fmt.Sprintf("%s/uploads/%s/blobs/%s/%d", instance, uuid.Must(uuid.NewRandom()), hex.EncodeToString(digest.GetHash()), digest.GetSizeBytes())
	}

	writeOffset := int64(0)
	for {
		readBuf := make([]byte, ba.readChunkSize)
		if n, err := r.Read(readBuf[:]); err == nil {
			// Non-terminating chunk.
			if err := client.Send(&bytestream.WriteRequest{
				ResourceName: resourceName,
				WriteOffset:  writeOffset,
				Data:         readBuf[:n],
			}); err != nil {
				return err
			}
			writeOffset += int64(n)
			resourceName = ""
		} else if err == io.EOF {
			// Terminating chunk.
			if err := client.Send(&bytestream.WriteRequest{
				ResourceName: resourceName,
				WriteOffset:  writeOffset,
				FinishWrite:  true,
				Data:         readBuf[:n],
			}); err != nil {
				return err
			}
			_, err := client.CloseAndRecv()
			return err
		} else {
			return err
		}
	}
}

func (ba *contentAddressableStorageBlobAccess) Delete(ctx context.Context, digest *util.Digest) error {
	return status.Error(codes.Unimplemented, "Bazel remote execution protocol does not support object deletion")
}

func (ba *contentAddressableStorageBlobAccess) FindMissing(ctx context.Context, digests []*util.Digest) ([]*util.Digest, error) {
	// Convert digests to line format.
	if len(digests) == 0 {
		return nil, nil
	}
	instance := digests[0].GetInstance()
	request := remoteexecution.FindMissingBlobsRequest{
		InstanceName: instance,
	}
	for _, digest := range digests {
		request.BlobDigests = append(request.BlobDigests, digest.GetRawDigest())
		if digest.GetInstance() != instance {
			return nil, status.Error(codes.InvalidArgument, "Cannot use mixed instance names in a single request")
		}
	}

	response, err := ba.contentAddressableStorageClient.FindMissingBlobs(ctx, &request)
	if err != nil {
		return nil, err
	}

	// Convert results back.
	var outDigests []*util.Digest
	for _, rawDigest := range response.MissingBlobDigests {
		digest, err := util.NewDigest(instance, rawDigest)
		if err != nil {
			return nil, err
		}
		outDigests = append(outDigests, digest)
	}
	return outDigests, nil
}
