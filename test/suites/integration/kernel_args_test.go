/*
Copyright (C) 2012-2025 Tencent. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package integration_test

import (
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	"github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"

	. "github.com/onsi/ginkgo/v2" //nolint:stylecheck
)

var _ = Describe("KernelArgs", func() {
	It("should apply kernel parameters to the provisioned node", func() {
		// Set kernel args in NodePool template annotations
		nodePool.Spec.Template.Annotations[v1beta1.AnnotationKernelArgPrefix+"vm.max_map_count"] = "262144"
		nodePool.Spec.Template.Annotations[v1beta1.AnnotationKernelArgPrefix+"net.core.somaxconn"] = "65535"
		nodePool.Spec.Template.Annotations[v1beta1.AnnotationKernelArgPrefix+"fs.file-max"] = "1048576"

		env.ExpectCreated(nodeClass, nodePool)

		// Create a test pod to trigger provisioning
		pod := env.TestPod()
		env.ExpectCreated(pod)

		// Wait for the pod to be healthy and a node to be created
		env.EventuallyExpectHealthy(pod)
		nodes := env.EventuallyExpectCreatedNodeCount("==", 1)
		env.EventuallyExpectInitializedNodeCount("==", 1)

		nodeName := nodes[0].Name

		// Verify kernel parameters
		env.ExpectKernelArg(nodeName, "vm.max_map_count", "262144")
		env.ExpectKernelArg(nodeName, "net.core.somaxconn", "65535")
		env.ExpectKernelArg(nodeName, "fs.file-max", "1048576")
	})
})

// suppress unused import warnings
var _ karpv1.NodePool
