// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"cloud.google.com/go/compute/metadata"
	sdlog "github.com/TV4/logrus-stackdriver-formatter"
	isatty "github.com/mattn/go-isatty"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/run/v2"
)

var (
	flLoggingLevel string
	flHTTPAddr     string
	flProject      string
)

func init() {
	defaultAddr := ":8080"
	if v := os.Getenv("PORT"); v != "" {
		defaultAddr = fmt.Sprintf(":%s", v)
	}

	flag.StringVar(&flLoggingLevel, "verbosity", "info", "the logging level (e.g. debug)")
	flag.StringVar(&flHTTPAddr, "http-addr", defaultAddr, "address where to listen to http requests (e.g. :8080)")
	flag.StringVar(&flProject, "project", "", "project in which the service is deployed")
	flag.Parse()

	args := flag.Args()
	if len(args) != 0 {
		logrus.Fatalf("positional arguments not accepted: %v", args)
	}
}

func main() {
	logger := logrus.New()
	loggingLevel, err := logrus.ParseLevel(flLoggingLevel)
	if err != nil {
		logger.Fatalf("invalid logging level: %v", err)
	}
	logger.SetLevel(loggingLevel)

	if !isatty.IsTerminal(os.Stdout.Fd()) {
		serviceName := os.Getenv("K_SERVICE")
		if serviceName == "" {
			serviceName = "serverless-autoneg-controller"
		}
		logger.Formatter = sdlog.NewFormatter(
			sdlog.WithService(serviceName),
		)
	}

	if flProject == "" {
		logger.Info("-project not specified, trying to autodetect one")
		flProject, err = determineProjectID(logger)
		if err != nil {
			logger.Fatalf("failed to detect project, must specify one with -project: %v", err)
		} else {
			logger.Infof("project detected: %s", flProject)
		}
	}

	ctx := context.Background()
	_, err = getCloudRunServices(ctx, logger, flProject, "europe-west1", "labe=xyz")

}

func getCloudRunServices(ctx context.Context, logger *logrus.Logger, project, region, labelSelector string) ([]*run.GoogleCloudRunV2Service, error) {
	lg := logger.WithFields(logrus.Fields{
		"region":        region,
		"labelSelector": labelSelector,
	})

	lg.Debug("querying Cloud Run services")
	runService, err := run.NewService(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize Cloud Run client")
	}

	svcs, err := runService.Projects.Locations.Services.List(fmt.Sprintf("projects/%s/locations/%s",project,region)).Do()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get services with label %q in region %q", labelSelector, region)
	}

	lg.WithField("n", len(svcs.Services)).Debug("finished retrieving services from the API")
	return svcs.Services, nil
}

func determineProjectID(logger *logrus.Logger) (string, error) {
	if metadata.OnGCE() {
		logger.Debug("trying gce metadata service for project ID")
		v, err := metadata.ProjectID()
		if err != nil {
			return "", errors.Wrapf(err, "error when getting project ID from compute metadata")
		}
		logger.Debug("found project ID on gce metadata")
		return v, nil
	}

	logger.Debug("service not running on gce, trying gcloud for core/project")
	cmd := exec.Command("gcloud", "config", "get-value", "core/project", "-q")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if err != nil {
		msg := "error when running gcloud command to get default project"
		if stderr.Len() != 0 {
			msg += fmt.Sprintf(", stderr=%s", stderr.String())
		}
		return "", errors.Wrap(err, msg)
	}

	v := strings.TrimSpace(stdout.String())
	if v == "" {
		return "", errors.New("gcloud command returned empty project value")
	}
	logger.Debug("found project ID on gcloud")
	return v, nil
}
