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

package v1beta1

import (
	"encoding/json"
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/runtime"
)

// RawExtensionFromProviderSpec marshals the machine provider spec.
func RawExtensionFromProviderSpec(spec *CXMMachineProviderSpec) (*runtime.RawExtension, error) {
	if spec == nil {
		return &runtime.RawExtension{}, nil
	}

	var rawBytes []byte
	var err error
	if rawBytes, err = json.Marshal(spec); err != nil {
		return nil, fmt.Errorf("error marshalling providerSpec: %v", err)
	}

	return &runtime.RawExtension{
		Raw: rawBytes,
	}, nil
}

// RawExtensionFromProviderStatus marshals the provider status
func RawExtensionFromProviderStatus(status *CXMMachineProviderStatus) (*runtime.RawExtension, error) {
	if status == nil {
		return &runtime.RawExtension{}, nil
	}

	var rawBytes []byte
	var err error
	if rawBytes, err = json.Marshal(status); err != nil {
		return nil, fmt.Errorf("error marshalling providerStatus: %v", err)
	}

	return &runtime.RawExtension{
		Raw: rawBytes,
	}, nil
}

// ProviderSpecFromRawExtension unmarshals the JSON-encoded spec
func ProviderSpecFromRawExtension(rawExtension *runtime.RawExtension) (*CXMMachineProviderSpec, error) {
	if rawExtension == nil {
		return &CXMMachineProviderSpec{}, nil
	}

	spec := new(CXMMachineProviderSpec)
	if err := json.Unmarshal(rawExtension.Raw, &spec); err != nil {
		return nil, fmt.Errorf("error unmarshalling providerSpec: %v", err)
	}

	return spec, nil
}

// ProviderStatusFromRawExtension unmarshals a raw extension into a CXMMachineProviderStatus type
func ProviderStatusFromRawExtension(rawExtension *runtime.RawExtension) (*CXMMachineProviderStatus, error) {
	if rawExtension == nil {
		return &CXMMachineProviderStatus{}, nil
	}

	providerStatus := new(CXMMachineProviderStatus)
	if err := json.Unmarshal(rawExtension.Raw, providerStatus); err != nil {
		return nil, fmt.Errorf("error unmarshalling providerStatus: %v", err)
	}

	return providerStatus, nil
}

func IsPrepaid(from interface{}) (bool, error) {
	if reflect.ValueOf(from).Kind() != reflect.Ptr {
		return false, fmt.Errorf("input object must be a pointer")
	}

	var providerSpec *CXMMachineProviderSpec
	switch obj := from.(type) {
	case *MachineSet:
		providerSpec, _ = ProviderSpecFromRawExtension(obj.Spec.Template.Spec.ProviderSpec.Value)
	case *Machine:
		providerSpec, _ = ProviderSpecFromRawExtension(obj.Spec.ProviderSpec.Value)
	case *CXMMachineProviderSpec:
		providerSpec = obj
	default:
	}

	if providerSpec == nil {
		return false, nil
	}

	return providerSpec.IsPrepaid() || providerSpec.IsUnderwrite(), nil
}
