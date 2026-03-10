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
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	"github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"
	"github.com/tencentcloud/karpenter-provider-tke/test/pkg/environment/tke"
)

var env *tke.Environment
var nodeClass *v1beta1.TKEMachineNodeClass
var nodePool *karpv1.NodePool

func TestIntegration(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	env = tke.NewEnvironment(t)
	ginkgo.RunSpecs(t, "Integration")
}

var _ = ginkgo.BeforeEach(func() {
	env.BeforeEach()
	nodeClass = env.DefaultTKEMachineNodeClass()
	nodePool = env.DefaultNodePool(nodeClass)
})

var _ = ginkgo.AfterEach(func() {
	env.Cleanup()
})

var _ = ginkgo.AfterEach(func() {
	env.AfterEach()
})
