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
	"github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"

	. "github.com/onsi/ginkgo/v2" //nolint:stylecheck
)

var _ = Describe("KubeletArgs", func() {
	It("should apply kubelet arguments to the provisioned node", func() {
		// Set kubelet args in NodePool template annotations
		nodePool.Spec.Template.Annotations[v1beta1.AnnotationKubeletArgPrefix+"max-pods"] = "200"

		env.ExpectCreated(nodeClass, nodePool)

		// Create a test pod to trigger provisioning
		pod := env.TestPod()
		env.ExpectCreated(pod)

		// Wait for the pod to be healthy and a node to be created
		env.EventuallyExpectHealthy(pod)
		nodes := env.EventuallyExpectCreatedNodeCount("==", 1)
		env.EventuallyExpectInitializedNodeCount("==", 1)

		nodeName := nodes[0].Name

		// Verify kubelet arguments are applied on the node
		env.ExpectKubeletArg(nodeName, "max-pods", "200")
	})
})
