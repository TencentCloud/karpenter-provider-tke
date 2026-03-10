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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:stylecheck
	. "github.com/onsi/gomega"    //nolint:stylecheck

	"github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("InvalidNodeClass", func() {
	It("should report Ready=False when NodeClass has an invalid subnet", func() {
		// Override nodeClass with an invalid subnet
		nodeClass.Spec.SubnetSelectorTerms = []v1beta1.SubnetSelectorTerm{
			{ID: "subnet-invalid"},
		}
		env.ExpectCreated(nodeClass)

		// Wait for the NodeClass status to show Ready=False with a message about subnet
		Eventually(func(g Gomega) {
			nc := &v1beta1.TKEMachineNodeClass{}
			g.Expect(env.Client.Get(env.Context, client.ObjectKeyFromObject(nodeClass), nc)).To(Succeed())

			conditions := nc.StatusConditions()
			readyCondition := conditions.Get(v1beta1.ConditionTypeNodeClassReady)
			g.Expect(readyCondition).ToNot(BeNil(),
				fmt.Sprintf("expected Ready condition on NodeClass %s", nc.Name))
			g.Expect(readyCondition.IsFalse()).To(BeTrue(),
				fmt.Sprintf("expected Ready condition to be False, got %v", readyCondition))
			g.Expect(readyCondition.Message).To(ContainSubstring("subnet"),
				fmt.Sprintf("expected Ready condition message to contain 'subnet', got %s", readyCondition.Message))
		}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
	})
})
