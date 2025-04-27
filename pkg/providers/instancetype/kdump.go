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

package instancetype

import "math"

type Kdump struct {
	low      int
	high     int
	reserved int
}

type Kdumps []Kdump

var KdumpLevels = Kdumps{
	{low: 1800 * 1024 * 1024, high: 64 * 1024 * 1024 * 1024, reserved: 256 * 1024 * 1024},
	{low: 64 * 1024 * 1024 * 1024, high: 128 * 1024 * 1024 * 1024, reserved: 512 * 1024 * 1024},
	{low: 128 * 1024 * 1024 * 1024, high: math.MaxInt32, reserved: 768 * 1024 * 1024},
}

func (k Kdump) eval(resource int) int {
	if resource < k.low || resource >= k.high {
		return 0
	}
	return k.reserved
}

func (ks Kdumps) eval(resource int) int {
	res := 0
	for _, level := range ks {
		res = level.eval(resource)
		if res > 0 {
			return res
		}
	}
	return res
}
