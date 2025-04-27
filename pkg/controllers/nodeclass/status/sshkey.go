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

package status

import (
	"context"
	"fmt"
	"sort"

	"time"

	sshkeyprovider "github.com/tencentcloud/karpenter-provider-tke/pkg/providers/sshkey"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/samber/lo"
	api "github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"
	cvm "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cvm/v20170312"
)

type SSHKey struct {
	sshKeyProvider sshkeyprovider.Provider
}

func (s *SSHKey) Reconcile(ctx context.Context, nodeClass *api.TKEMachineNodeClass) (reconcile.Result, error) {
	sshkeys, err := s.sshKeyProvider.List(ctx, nodeClass)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("getting ssh key pairs, %w", err)
	}
	if len(sshkeys) == 0 {
		nodeClass.Status.SSHKeys = nil
		return reconcile.Result{}, nil
	}
	sort.Slice(sshkeys, func(i, j int) bool {
		return *sshkeys[i].KeyId < *sshkeys[j].KeyId
	})
	nodeClass.Status.SSHKeys = lo.Map(sshkeys, func(k *cvm.KeyPair, _ int) api.SSHKey {
		return api.SSHKey{
			ID: lo.FromPtr(k.KeyId),
		}
	})

	return reconcile.Result{RequeueAfter: time.Minute}, nil
}
