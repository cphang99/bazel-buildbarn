package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"time"

	"github.com/EdSchouten/bazel-buildbarn/pkg/ac"
	"github.com/EdSchouten/bazel-buildbarn/pkg/blobstore"
	"github.com/EdSchouten/bazel-buildbarn/pkg/blobstore/configuration"
	"github.com/EdSchouten/bazel-buildbarn/pkg/builder"
	"github.com/EdSchouten/bazel-buildbarn/pkg/cas"
	"github.com/EdSchouten/bazel-buildbarn/pkg/environment"
	"github.com/EdSchouten/bazel-buildbarn/pkg/filesystem"
	"github.com/EdSchouten/bazel-buildbarn/pkg/proto/scheduler"
	"github.com/EdSchouten/bazel-buildbarn/pkg/util"
	"github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"google.golang.org/grpc"
)

func main() {
	var (
		blobstoreConfig    = flag.String("blobstore-config", "/config/blobstore.conf", "Configuration for blob storage")
		browserURLString   = flag.String("browser-url", "http://bbb-browser/", "URL of the Bazel Buildbarn Browser, accessible by the user through 'bazel build --verbose_failures'")
		buildDirectoryPath = flag.String("build-directory", "/build", "Directory where builds take place")
		cacheDirectoryPath = flag.String("cache-directory", "/cache", "Directory where build input files are cached")
		concurrency        = flag.Int("concurrency", 1, "Number of actions to run concurrently")
		runnerAddress      = flag.String("runner", "", "Address of the runner to which to connect")
		schedulerAddress   = flag.String("scheduler", "", "Address of the scheduler to which to connect")
		webListenAddress   = flag.String("web.listen-address", ":80", "Port on which to expose metrics")
	)
	flag.Parse()

	browserURL, err := url.Parse(*browserURLString)
	if err != nil {
		log.Fatal("Failed to parse browser URL: ", err)
	}

	// Web server for metrics and profiling.
	http.Handle("/metrics", promhttp.Handler())
	go func() {
		log.Fatal(http.ListenAndServe(*webListenAddress, nil))
	}()

	// Storage access.
	contentAddressableStorageBlobAccess, actionCacheBlobAccess, err := configuration.CreateBlobAccessObjectsFromConfig(*blobstoreConfig)
	if err != nil {
		log.Fatal("Failed to create blob access: ", err)
	}

	// Directories where builds take place.
	buildDirectory, err := filesystem.NewLocalDirectory(*buildDirectoryPath)
	if err != nil {
		log.Fatal("Failed to open cache directory: ", err)
	}

	// On-disk caching of content for efficient linking into build environments.
	cacheDirectory, err := filesystem.NewLocalDirectory(*cacheDirectoryPath)
	if err != nil {
		log.Fatal("Failed to open cache directory: ", err)
	}
	if err := cacheDirectory.RemoveAllChildren(); err != nil {
		log.Fatal("Failed to clear cache directory: ", err)
	}

	// Cached read access to the Content Addressable Storage. All
	// workers make use of the same cache, to increase the hit rate.
	contentAddressableStorageReader := cas.NewDirectoryCachingContentAddressableStorage(
		cas.NewHardlinkingContentAddressableStorage(
			cas.NewBlobAccessContentAddressableStorage(
				blobstore.NewExistencePreconditionBlobAccess(contentAddressableStorageBlobAccess)),
			util.DigestKeyWithoutInstance, cacheDirectory, 10000, 1<<30),
		util.DigestKeyWithoutInstance, 1000)
	actionCache := ac.NewBlobAccessActionCache(actionCacheBlobAccess)

	// Create connection with scheduler.
	schedulerConnection, err := grpc.Dial(
		*schedulerAddress,
		grpc.WithInsecure(),
		grpc.WithUnaryInterceptor(grpc_prometheus.UnaryClientInterceptor),
		grpc.WithStreamInterceptor(grpc_prometheus.StreamClientInterceptor))
	if err != nil {
		log.Fatal("Failed to create scheduler RPC client: ", err)
	}
	schedulerClient := scheduler.NewSchedulerClient(schedulerConnection)

	// Execute commands using a separate runner process. Due to the
	// interaction between threads, forking and execve() returning
	// ETXTBSY, concurrent execution of build actions can only be
	// used in combination with a runner process. Having a separate
	// runner process also makes it possible to apply privilege
	// separation.
	runnerConnection, err := grpc.Dial(
		*runnerAddress,
		grpc.WithInsecure(),
		grpc.WithUnaryInterceptor(grpc_prometheus.UnaryClientInterceptor),
		grpc.WithStreamInterceptor(grpc_prometheus.StreamClientInterceptor))
	if err != nil {
		log.Fatal("Failed to create runner RPC client: ", err)
	}

	// Create a per-action directory named after the action digest, so that
	// multiple actions may be run concurrently within the same environment.
	environmentManager := environment.NewActionDigestSubdirectoryManager(
		environment.NewSingletonManager(
			environment.NewRemoteExecutionEnvironment(runnerConnection, buildDirectory)),
		util.DigestKeyWithoutInstance)

	for i := 0; i < *concurrency; i++ {
		go func(i int) {
			// Per-worker separate writer of the Content
			// Addressable Storage that batches writes after
			// completing the build action.
			contentAddressableStorageWriter, contentAddressableStorageFlusher := blobstore.NewBatchedStoreBlobAccess(
				blobstore.NewExistencePreconditionBlobAccess(contentAddressableStorageBlobAccess),
				util.DigestKeyWithoutInstance, 100)
			contentAddressableStorageWriter = blobstore.NewMetricsBlobAccess(
				contentAddressableStorageWriter,
				"cas_batched_store")
			contentAddressableStorage := cas.NewReadWriteDecouplingContentAddressableStorage(
				contentAddressableStorageReader,
				cas.NewBlobAccessContentAddressableStorage(contentAddressableStorageWriter))
			buildExecutor := builder.NewStorageFlushingBuildExecutor(
				builder.NewCachingBuildExecutor(
					builder.NewLocalBuildExecutor(
						contentAddressableStorage,
						environmentManager),
					contentAddressableStorage,
					actionCache,
					browserURL),
				contentAddressableStorageFlusher)

			// Repeatedly ask the scheduler for work.
			for {
				err := subscribeAndExecute(schedulerClient, buildExecutor, browserURL)
				log.Print("Failed to subscribe and execute: ", err)
				time.Sleep(time.Second * 3)
			}
		}(i)
	}
	select {}
}

func subscribeAndExecute(schedulerClient scheduler.SchedulerClient, buildExecutor builder.BuildExecutor, browserURL *url.URL) error {
	stream, err := schedulerClient.GetWork(context.Background())
	if err != nil {
		return err
	}
	defer stream.CloseSend()

	for {
		request, err := stream.Recv()
		if err != nil {
			return err
		}

		// Print URL of the action into the log before execution.
		actionURL, err := browserURL.Parse(
			fmt.Sprintf(
				"/action/%s/%s/%d/",
				request.InstanceName,
				request.ActionDigest.Hash,
				request.ActionDigest.SizeBytes))
		if err != nil {
			return err
		}
		log.Print("Action: ", actionURL.String())

		response, _ := buildExecutor.Execute(stream.Context(), request)
		log.Print("ExecuteResponse: ", response)
		if err := stream.Send(response); err != nil {
			return err
		}
	}
}
