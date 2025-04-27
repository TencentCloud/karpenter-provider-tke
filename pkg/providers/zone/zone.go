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

package zone

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

type Provider interface {
	ZoneFromID(string) (string, error)
	IDFromZone(string) (string, error)
}

type DefaultProvider struct {
	zoneGroups map[string]int
}

func NewDefaultProvider(_ context.Context) *DefaultProvider {
	return &DefaultProvider{
		zoneGroups: map[string]int{
			"ap-guangzhou":       100000,
			"ap-shenzhen-fsi":    110000,
			"ap-guangzhou-open":  120000,
			"na-siliconvalley":   150000,
			"ap-chengdu":         160000,
			"eu-frankfurt":       170000,
			"ap-seoul":           180000,
			"ap-chongqing":       190000,
			"ap-shanghai":        200000,
			"ap-mumbai":          210000,
			"na-ashburn":         220000,
			"ap-bangkok":         230000,
			"ap-tokyo":           250000,
			"ap-hongkong":        300000,
			"ap-jinan-ec":        310000,
			"ap-hangzhou-ec":     320000,
			"ap-nanjing":         330000,
			"ap-fuzhou-ec":       340000,
			"ap-wuhan-ec":        350000,
			"ap-tianjin":         360000,
			"ap-shenzhen":        370000,
			"ap-taipei":          390000,
			"na-toronto":         400000,
			"ap-changsha-ec":     450000,
			"ap-beijing-fsi":     460000,
			"ap-shijiazhuang-ec": 530000,
			"ap-qingyuan":        540000,
			"ap-hefei-ec":        550000,
			"ap-shenyang-ec":     560000,
			"ap-xian-ec":         570000,
			"ap-xibei-ec":        580000,
			"ap-shanghai-fsi":    700000,
			"ap-zhengzhou-ec":    710000,
			"ap-jakarta":         720000,
			"sa-saopaulo":        740000,
			"ap-shanghai-adc":    780000,
			"ap-beijing":         800000,
			"ap-guangzhou-wxzf":  820000,
			"ap-shanghai-wxzf":   830000,
			"ap-singapore":       900000,
		},
	}
}

func (p *DefaultProvider) ZoneFromID(id string) (string, error) {
	idNum, err := strconv.Atoi(id)
	if err != nil {
		return "", fmt.Errorf("failed to convert id: %v", err)
	}

	for ap, apNum := range p.zoneGroups {
		if idNum > apNum && idNum < apNum+1000 {
			return fmt.Sprintf("%s-%d", ap, idNum-apNum), nil
		}
	}
	return "", fmt.Errorf("failed to find zone for id %s", id)
}

func (p *DefaultProvider) IDFromZone(zone string) (string, error) {
	zoneSlice := strings.Split(zone, "-")

	if len(zoneSlice) < 3 {
		return "", fmt.Errorf("invalid zone %s", zone)
	}
	if len(zoneSlice) == 4 {
		zoneSlice[0] = zoneSlice[0] + "-" + zoneSlice[1] + "-" + zoneSlice[2]
		zoneSlice[2] = zoneSlice[3]
	} else {
		zoneSlice[0] = zoneSlice[0] + "-" + zoneSlice[1]
	}

	idNum, err := strconv.Atoi(zoneSlice[2])
	if err != nil {
		return "", fmt.Errorf("invalid zone %s", zone)
	}

	if apNum, ok := p.zoneGroups[zoneSlice[0]]; ok {
		return strconv.Itoa(apNum + idNum), nil
	}

	return "", fmt.Errorf("failed to find zone %s", zone)
}
