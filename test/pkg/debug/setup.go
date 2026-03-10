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

package debug

import (
	"context"

	"github.com/onsi/ginkgo/v2"
	"github.com/samber/lo"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega" //nolint:stylecheck
)

const (
	NoWatch  = "NoWatch"
	NoEvents = "NoEvents"
)

var m *Monitor
var e *EventClient

func BeforeEach(ctx context.Context, config *rest.Config, kubeClient client.Client) {
	if !lo.Contains(ginkgo.CurrentSpecReport().Labels(), NoWatch) {
		m = New(ctx, config, kubeClient)
		m.MustStart()
	}
	if !lo.Contains(ginkgo.CurrentSpecReport().Labels(), NoEvents) {
		e = NewEventClient(kubeClient)
	}
}

func AfterEach(ctx context.Context) {
	if !lo.Contains(ginkgo.CurrentSpecReport().Labels(), NoWatch) {
		m.Stop()
	}
	if !lo.Contains(ginkgo.CurrentSpecReport().Labels(), NoEvents) {
		Expect(e.DumpEvents(ctx)).To(Succeed())
	}
}
