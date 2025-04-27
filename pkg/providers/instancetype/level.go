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

type Level struct {
	low  int64
	high int64
	base int64
	fact float64
}

func (l Level) eval(resource int64) float64 {
	if resource < l.low {
		return 0
	}
	if resource < l.high {
		return float64(resource-l.low)*l.fact + float64(l.base)
	}
	return float64(l.high-l.low)*l.fact + float64(l.base)
}

type Levels []Level

func (ls Levels) eval(fraction int64) int64 {
	res := 0.0
	for _, level := range ls {
		res += level.eval(fraction)
	}
	return int64(res)
}
