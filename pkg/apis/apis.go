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

package apis

import (
	"k8s.io/apimachinery/pkg/runtime"

	api "github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"
	capiv1beta1 "github.com/tencentcloud/karpenter-provider-tke/staging/nativenode/v1beta1"
)

var (
	// Builder includes all types within the apis package
	Builder = runtime.NewSchemeBuilder(
		api.SchemeBuilder.AddToScheme,
		capiv1beta1.AddToScheme,
	)
	// AddToScheme may be used to add all resources defined in the project to a Scheme
	AddToScheme = Builder.AddToScheme
)

//go:generate controller-gen crd object:headerFile="../../hack/boilerplate.go.txt" paths="./..." output:crd:artifacts:config=crds
var ()
