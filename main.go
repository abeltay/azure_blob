package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/Azure/azure-pipeline-go/pipeline"
	"github.com/Azure/azure-storage-blob-go/azblob"
)

const (
	accountName   = "account_name"
	accountKey    = "account_key"
	serviceName   = "https://something.blob.core.windows.net/"
	containerName = "container"
)

func main() {
	// Create/configure a request pipeline options object.
	// All PipelineOptions' fields are optional; reasonable defaults are set for anything you do not specify
	po := azblob.PipelineOptions{
		// Set RetryOptions to control how HTTP request are retried when retryable failures occur
		Retry: azblob.RetryOptions{
			Policy:        azblob.RetryPolicyExponential, // Use exponential backoff as opposed to linear
			MaxTries:      3,                             // Try at most 3 times to perform the operation (set to 1 to disable retries)
			TryTimeout:    time.Second * 3,               // Maximum time allowed for any single try
			RetryDelay:    time.Second * 1,               // Backoff amount for each retry (exponential or linear)
			MaxRetryDelay: time.Second * 3,               // Max delay between retries
		},

		// Set RequestLogOptions to control how each HTTP request & its response is logged
		RequestLog: azblob.RequestLogOptions{
			LogWarningIfTryOverThreshold: time.Millisecond * 200, // A successful response taking more than this time to arrive is logged as a warning
		},

		// Set HTTPSender to override the default HTTP Sender that sends the request over the network
		HTTPSender: pipeline.FactoryFunc(func(next pipeline.Policy, po *pipeline.PolicyOptions) pipeline.PolicyFunc {
			return func(ctx context.Context, request pipeline.Request) (pipeline.Response, error) {
				// Implement the HTTP client that will override the default sender.
				// For example, below HTTP client uses a transport that is different from http.DefaultTransport
				client := http.Client{
					Transport: &http.Transport{
						Proxy: nil,
						DialContext: (&net.Dialer{
							Timeout:   30 * time.Second,
							KeepAlive: 30 * time.Second,
							DualStack: true,
						}).DialContext,
						MaxIdleConns:          100,
						IdleConnTimeout:       180 * time.Second,
						TLSHandshakeTimeout:   10 * time.Second,
						ExpectContinueTimeout: 1 * time.Second,
					},
				}

				// Send the request over the network
				resp, err := client.Do(request.WithContext(ctx))

				return pipeline.NewHTTPResponse(resp), err
			}
		}),
	}

	credential, err := azblob.NewSharedKeyCredential(accountName, accountKey)
	if err != nil {
		log.Println(err)
		return
	}

	// Create a request pipeline object configured with credentials and with pipeline options. Once created,
	// a pipeline object is goroutine-safe and can be safely used with many XxxURL objects simultaneously.
	p := azblob.NewPipeline(credential, po) // A pipeline always requires some credential object

	// Once you've created a pipeline object, associate it with an XxxURL object so that you can perform HTTP requests with it.
	u, _ := url.Parse(serviceName)
	serviceURL := azblob.NewServiceURL(*u, p)
	// Use the serviceURL as desired...

	// NOTE: When you use an XxxURL object to create another XxxURL object, the new XxxURL object inherits the
	// same pipeline object as its parent. For example, the containerURL and blobURL objects (created below)
	// all share the same pipeline. Any HTTP operations you perform with these objects share the behavior (retry, logging, etc.)
	containerURL := serviceURL.NewContainerURL(containerName)
	blobURL := containerURL.NewBlockBlobURL("test.txt")
	ctx := context.Background()

	file, err := os.Open("sample.txt")
	if err != nil {
		log.Println(err)
		return
	}
	_, err = blobURL.Upload(ctx, file, azblob.BlobHTTPHeaders{}, azblob.Metadata{}, azblob.BlobAccessConditions{}, azblob.AccessTierNone, nil)
	if err != nil {
		log.Println(err)
		return
	}

	resp, err := blobURL.Download(ctx, 0, 0, azblob.BlobAccessConditions{}, false)
	if err != nil {
		log.Println(err)
		return
	}
	body := resp.Body(azblob.RetryReaderOptions{})
	defer body.Close()

	b, err := ioutil.ReadAll(body)
	if err != nil {
		log.Println(err)
		return
	}
	fmt.Printf("%s", b)

	_, err = blobURL.Delete(ctx, azblob.DeleteSnapshotsOptionInclude, azblob.BlobAccessConditions{})
	if err != nil {
		log.Println(err)
		return
	}
}
