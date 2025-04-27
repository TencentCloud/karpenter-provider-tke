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

package cxm

type InstanceTypeQuotaItem struct {
	Zone              string  `json:"Zone"`
	InstanceFamily    string  `json:"InstanceFamily"`
	InstanceType      string  `json:"InstanceType"`
	CPU               int     `json:"Cpu"`
	CpuType           string  `json:"CpuType"`
	Memory            int     `json:"Memory"`
	Fpga              int     `json:"Fpga"`
	Gpu               int     `json:"Gpu"`
	GpuCount          float64 `json:"GpuCount"`
	Inventory         int     `json:"Inventory"`
	SpotpaidInventory *int    `json:"SpotpaidInventory"`
	InstanceQuota     int     `json:"InstanceQuota"`

	Status            string          `json:"Status"`
	Remark            string          `json:"Remark"`
	Externals         Externals       `json:"Externals"`
	LocalDiskTypeList []LocalDiskType `json:"LocalDiskTypeList,omitempty"  name:"LocalDiskTypeList"`
	TypeName          string          `json:"TypeName,omitempty"           name:"TypeName"`
	InstanceBandwidth float64         `json:"InstanceBandwidth"`
	StorageBlock      uint64          `json:"StorageBlock,omitempty"       name:"StorageBlock"`
	Price             ItemPrice       `json:"Price,omitempty"`
}

type Externals struct {
	ReleaseAddress           *bool        `json:"ReleaseAddress,omitempty"    name:"ReleaseAddress"`
	GpuAttr                  GpuAttr      `json:"GpuAttr"`
	GPUDesc                  string       `json:"GPUDesc"`
	UnsupportNetworks        []string     `json:"UnsupportNetworks,omitempty" name:"UnsupportNetworks"`
	StorageBlockAttr         StorageBlock `json:"StorageBlockAttr,omitempty"  name:"StorageBlockAttr"`
	PrepaidUnderwriteEnable  bool         `json:"prepaid_underwrite_enable,omitempty"`
	PrepaidUnderwritePeriods []int        `json:"prepaid_underwrite_periods,omitempty"`
}

type LocalDiskType struct {
	Type          string `json:"Type,omitempty"          name:"Type"`
	PartitionType string `json:"PartitionType,omitempty" name:"PartitionType"`
	MinSize       int64  `json:"MinSize,omitempty"       name:"MinSize"`
	MaxSize       int64  `json:"MaxSize,omitempty"       name:"MaxSize"`
}

type ItemPrice struct {
	UnitPrice     float64  `json:"UnitPrice,omitempty"		name:"UnitPrice"`
	OriginalPrice float64  `json:"OriginalPrice,omitempty"	name:"OriginalPrice"`
	SpotpaidPrice *float64 `json:"SpotpaidPrice,omitempty"	name:"SpotpaidPrice"`
}

type StorageBlock struct {
	Type    string `json:"Type,omitempty"    name:"Type"`
	MinSize int64  `json:"MinSize,omitempty" name:"MinSize"`
	MaxSize int64  `json:"MaxSize,omitempty" name:"MaxSize"`
}

type GpuAttr struct {
	Ratio float64 `json:"Ratio"`
	Type  string  `json:"Type"`
}
