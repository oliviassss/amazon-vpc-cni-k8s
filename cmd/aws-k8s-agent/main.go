// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

// The aws-node ipam daemon binary
package main

import (
	"os"
	"time"

	"github.com/aws/amazon-vpc-cni-k8s/pkg/ipamd"
	"github.com/aws/amazon-vpc-cni-k8s/pkg/k8sapi"
	"github.com/aws/amazon-vpc-cni-k8s/pkg/utils/eventrecorder"
	"github.com/aws/amazon-vpc-cni-k8s/pkg/utils/logger"
	"github.com/aws/amazon-vpc-cni-k8s/pkg/version"
	"github.com/aws/amazon-vpc-cni-k8s/utils"
	metrics "github.com/aws/amazon-vpc-cni-k8s/utils/prometheusmetrics"
)

const (
	appName = "aws-node"
	// metricsPort is the port for prometheus metrics
	metricsPort = 61678

	// Environment variable to disable the metrics endpoint on 61678
	envDisableMetrics = "DISABLE_METRICS"

	// Environment variable to disable the IPAMD introspection endpoint on 61679
	envDisableIntrospection = "DISABLE_INTROSPECTION"

	pollInterval = 5 * time.Second
	pollTimeout  = 30 * time.Second
)

func main() {
	os.Exit(_main())
}

func _main() int {
	// Do not add anything before initializing logger
	log := logger.Get()

	log.Infof("Starting L-IPAMD %s  ...", version.Version)
	version.RegisterMetric()

	enabledPodEni := ipamd.EnablePodENI()
	enabledCustomNetwork := ipamd.UseCustomNetworkCfg()
	withApiSever := false
	// Check API Server Connectivity
	if enabledPodEni || enabledCustomNetwork {
		if err := k8sapi.CheckAPIServerConnectivity(); err != nil {
			log.Errorf("Failed to check API server connectivity: %s", err)
			return 1
		} else {
			log.Info("API server connectivity established.")
			withApiSever = true
		}
	} else {
		log.Info("Waiting up to 30s for API server connectivity...")
		if err := k8sapi.CheckAPIServerConnectivityWithTimeout(pollInterval, pollTimeout); err != nil {
			log.Warn("Proceeding without API server connectivity")
			withApiSever = false
		} else {
			log.Info("API server connectivity established.")
			withApiSever = true
		}
	}
	log.Info("------------------ here")
	// Create Kubernetes client for API server requests
	k8sClient, err := k8sapi.CreateKubeClient(appName)
	if err != nil {
		log.Errorf("Failed to create kube client: %s", err)
	}
	log.Info("------------------ here1")
	// Create EventRecorder for use by IPAMD
	if err := eventrecorder.Init(k8sClient, withApiSever); err != nil {
		log.Errorf("Failed to create event recorder: %s", err)
		log.Warn("Skipping event recorder initialization")
		//return 1
	}
	log.Info("------------------ here2")
	ipamContext, err := ipamd.New(k8sClient, withApiSever)
	if err != nil {
		log.Errorf("---------------------------- Initialization failure: %v", err)
		return 1
	}

	// Pool manager
	go ipamContext.StartNodeIPPoolManager()

	if !utils.GetBoolAsStringEnvVar(envDisableMetrics, false) {
		// Prometheus metrics
		go metrics.ServeMetrics(metricsPort)
	}

	// CNI introspection endpoints
	if !utils.GetBoolAsStringEnvVar(envDisableIntrospection, false) {
		go ipamContext.ServeIntrospection()
	}

	// Start the RPC listener
	err = ipamContext.RunRPCHandler(version.Version)
	if err != nil {
		log.Errorf("Failed to set up gRPC handler: %v", err)
		return 1
	}
	return 0
}
