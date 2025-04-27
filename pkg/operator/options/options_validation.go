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

package options

import (
	"fmt"

	"go.uber.org/multierr"
)

func (o Options) Validate() error {
	return multierr.Combine(
		o.validateVMMemoryOverheadPercent(),
		o.validateRequiredFields(),
	)
}

func (o Options) validateRequiredFields() error {
	if o.Region == "" {
		return fmt.Errorf("missing field, region")
	}
	if o.ClusterID == "" {
		return fmt.Errorf("missing field, cluster-id")
	}
	if o.SecretID == "" {
		return fmt.Errorf("missing field, secret-id")
	}
	if o.SecretKey == "" {
		return fmt.Errorf("missing field, secret-key")
	}
	return nil
}

func (o Options) validateVMMemoryOverheadPercent() error {
	if o.VMMemoryOverheadPercent < 0 {
		return fmt.Errorf("vm-memory-overhead-percent cannot be negative")
	}
	return nil
}
