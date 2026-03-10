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

package tke

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/kubernetes/scheme"

	corev1 "k8s.io/api/core/v1"

	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	coretest "sigs.k8s.io/karpenter/pkg/test"

	"github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"
	"github.com/tencentcloud/karpenter-provider-tke/test/pkg/environment/common"
)

func init() {
	_ = v1beta1.AddToScheme(scheme.Scheme)
}

type Environment struct {
	*common.Environment

	Region          string
	SubnetID        string
	SecurityGroupID string
	SSHKeyID        string
	ZoneID          string
	Zone            string
	ClusterName     string
	CCRPrefix       string
}

func NewEnvironment(t *testing.T) *Environment {
	env := &Environment{
		Environment: common.NewEnvironment(t),
	}
	env.loadConfig()

	// Override default image to use CCR registry
	if env.CCRPrefix != "" {
		coretest.DefaultImage = fmt.Sprintf("%s.ccs.tencentyun.com/library/pause:latest", env.CCRPrefix)
	}

	return env
}

func (env *Environment) loadConfig() {
	// Priority: environment variables > cache file
	env.Region = os.Getenv("REGION")
	env.SubnetID = os.Getenv("SUBNET_ID")
	env.SecurityGroupID = os.Getenv("SECURITY_GROUP_ID")
	env.SSHKeyID = os.Getenv("SSH_KEY_ID")
	env.ZoneID = os.Getenv("ZONE_ID")
	env.Zone = os.Getenv("ZONE")
	env.ClusterName = os.Getenv("CLUSTER_NAME")
	env.CCRPrefix = os.Getenv("CCR_PREFIX")

	// If required vars are missing, try loading from cache file.
	// go test sets the working directory to the test package dir, so we
	// search several candidate paths relative to the project root.
	if env.Region == "" || env.SubnetID == "" || env.SecurityGroupID == "" || env.SSHKeyID == "" || env.ZoneID == "" {
		for _, candidate := range []string{
			"test/integration/env.cache",            // project root
			"../../../test/integration/env.cache",   // from test/suites/integration/
			"../../test/integration/env.cache",      // from test/suites/
			"../../../../test/integration/env.cache", // deeper nesting
		} {
			env.loadFromCacheFile(candidate)
			if env.Region != "" && env.SubnetID != "" && env.SecurityGroupID != "" && env.SSHKeyID != "" && env.ZoneID != "" {
				break
			}
		}
	}

	// Extract cluster name from kubeconfig if not set
	if env.ClusterName == "" {
		env.ClusterName = extractClusterNameFromConfig(env.Config.Host)
	}

	// If required vars are still missing, attempt auto-discovery from cluster
	if env.Region == "" || env.SubnetID == "" || env.SecurityGroupID == "" || env.SSHKeyID == "" || env.ZoneID == "" {
		env.discoverFromCluster()
	}

	if env.Region == "" || env.SubnetID == "" || env.SecurityGroupID == "" || env.SSHKeyID == "" || env.ZoneID == "" {
		panic("missing required environment variables: REGION, SUBNET_ID, SECURITY_GROUP_ID, SSH_KEY_ID, ZONE_ID. " +
			"Set them via environment variables or ensure test/integration/env.cache exists.")
	}
}

func (env *Environment) loadFromCacheFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), "\"")

		switch key {
		case "REGION":
			if env.Region == "" {
				env.Region = value
			}
		case "SUBNET_ID":
			if env.SubnetID == "" {
				env.SubnetID = value
			}
		case "SECURITY_GROUP_ID":
			if env.SecurityGroupID == "" {
				env.SecurityGroupID = value
			}
		case "SSH_KEY_ID":
			if env.SSHKeyID == "" {
				env.SSHKeyID = value
			}
		case "ZONE_ID":
			if env.ZoneID == "" {
				env.ZoneID = value
			}
		case "ZONE":
			if env.Zone == "" {
				env.Zone = value
			}
		case "CCR_PREFIX":
			if env.CCRPrefix == "" {
				env.CCRPrefix = value
			}
		}
	}
}

func extractClusterNameFromConfig(host string) string {
	// Extract cls-xxx from kubeconfig server URL
	idx := strings.Index(host, "cls-")
	if idx == -1 {
		return ""
	}
	end := idx
	for end < len(host) && host[end] != '.' && host[end] != '/' && host[end] != ':' {
		end++
	}
	return host[idx:end]
}

// DefaultTKEMachineNodeClass creates a TKEMachineNodeClass with the correct subnet/sg/sshkey
// and no InternetAccessible (to avoid creating public network resources).
func (env *Environment) DefaultTKEMachineNodeClass() *v1beta1.TKEMachineNodeClass {
	return &v1beta1.TKEMachineNodeClass{
		ObjectMeta: coretest.ObjectMeta(),
		Spec: v1beta1.TKEMachineNodeClassSpec{
			SubnetSelectorTerms: []v1beta1.SubnetSelectorTerm{
				{ID: env.SubnetID},
			},
			SecurityGroupSelectorTerms: []v1beta1.SecurityGroupSelectorTerm{
				{ID: env.SecurityGroupID},
			},
			SSHKeySelectorTerms: []v1beta1.SSHKeySelectorTerm{
				{ID: env.SSHKeyID},
			},
			// No InternetAccessible to avoid creating public network resources
		},
	}
}

// DefaultNodePool creates a standard NodePool pointing to the given TKEMachineNodeClass.
func (env *Environment) DefaultNodePool(nodeClass *v1beta1.TKEMachineNodeClass) *karpv1.NodePool {
	return coretest.NodePool(karpv1.NodePool{
		Spec: karpv1.NodePoolSpec{
			Template: karpv1.NodeClaimTemplate{
				ObjectMeta: karpv1.ObjectMeta{
					Labels: map[string]string{
						"karpenter-test":        "true",
						coretest.DiscoveryLabel: "unspecified",
					},
					Annotations: map[string]string{},
				},
				Spec: karpv1.NodeClaimTemplateSpec{
					NodeClassRef: &karpv1.NodeClassReference{
						Group: v1beta1.SchemeGroupVersion.Group,
						Kind:  "TKEMachineNodeClass",
						Name:  nodeClass.Name,
					},
					Requirements: []karpv1.NodeSelectorRequirementWithMinValues{
						{
							Key:      "kubernetes.io/os",
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{"linux"},
						},
						{
							Key:      karpv1.CapacityTypeLabelKey,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{karpv1.CapacityTypeOnDemand},
						},
					},
					Taints: []corev1.Taint{
						{
							Key:    "karpenter-test",
							Value:  "true",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
					ExpireAfter: karpv1.MustParseNillableDuration("Never"),
				},
			},
			Limits: karpv1.Limits(corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("10"),
			}),
			Disruption: karpv1.Disruption{
				ConsolidateAfter: karpv1.MustParseNillableDuration("30s"),
			},
		},
	})
}
