// Copyright (c) 2020 tickstep.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package command

import (
	"fmt"
	"github.com/olekukonko/tablewriter"
	"github.com/phpc0de/ctapi/cloudpan"
	"github.com/phpc0de/ctpango/cmder"
	"github.com/phpc0de/ctpango/cmder/cmdtable"
	"github.com/phpc0de/ctpango/internal/config"
	"github.com/urfave/cli"
	"strconv"
	"strings"
)

func CmdFamily() cli.Command {
	return cli.Command{
		Name:  "family",
		Usage: "切换云工作模式（家庭云/个人云）",
		Description: `
	切换已登录的天翼帐号的云工作模式（家庭云/个人云）
	如果运行该条命令没有提供参数, 程序将会列出所有的家庭云, 供选择切换.

	示例:
	cloudpan189-go family
	cloudpan189-go family <familyId>
`,
		Category: "天翼云盘账号",
		Before:   cmder.ReloadConfigFunc,
		After:    cmder.SaveConfigFunc,
		Action: func(c *cli.Context) error {
			inputData := c.Args().Get(0)
			targetFamilyId := int64(-1)
			if inputData != "" && len(inputData) > 0 {
				targetFamilyId, _ = strconv.ParseInt(inputData, 10, 0)
			}
			RunSwitchFamilyList(targetFamilyId)
			return nil
		},
	}
}

func RunSwitchFamilyList(targetFamilyId int64)  {
	currentFamilyId := config.Config.ActiveUser().ActiveFamilyId
	var activeFamilyInfo *cloudpan.AppFamilyInfo = nil
	familyList,renderStr := getFamilyOptionList()

	if familyList == nil || len(familyList) == 0 {
		fmt.Println("切换云工作模式失败")
		return
	}

	if targetFamilyId < 0 {
		// show option list
		fmt.Println(renderStr)

		// 提示输入 index
		var index string
		fmt.Printf("输入要切换的家庭云 # 值 > ")
		_, err := fmt.Scanln(&index)
		if err != nil {
			return
		}

		if n, err := strconv.Atoi(index); err == nil && n >= 0 && n < len(familyList) {
			activeFamilyInfo = familyList[n]
		} else {
			fmt.Printf("切换云工作模式失败, 请检查 # 值是否正确\n")
			return
		}
	} else {
		// 直接切换
		for _,familyInfo := range familyList {
			if familyInfo.FamilyId == targetFamilyId {
				activeFamilyInfo = familyInfo
				break
			}
		}
	}

	if activeFamilyInfo == nil {
		fmt.Printf("切换云工作模式失败\n")
		return
	}

	config.Config.ActiveUser().ActiveFamilyId = activeFamilyInfo.FamilyId
	config.Config.ActiveUser().ActiveFamilyInfo = *activeFamilyInfo
	if currentFamilyId != config.Config.ActiveUser().ActiveFamilyId {
		// clear the family work path
		config.Config.ActiveUser().FamilyWorkdir = "/"
		config.Config.ActiveUser().FamilyWorkdirFileEntity = *cloudpan.NewAppFileEntityForRootDir()
	}
	if activeFamilyInfo.FamilyId > 0 {
		fmt.Printf("切换云工作模式：家庭云 %s\n", activeFamilyInfo.RemarkName)
	} else {
		fmt.Printf("切换云工作模式：%s\n", activeFamilyInfo.RemarkName)
	}

}

func getFamilyOptionList() ([]*cloudpan.AppFamilyInfo, string) {
	activeUser := config.Config.ActiveUser()

	familyResult,err := activeUser.PanClient().AppFamilyGetFamilyList()
	if err != nil {
		fmt.Println("获取家庭列表失败")
		return nil, ""
	}
	t := []*cloudpan.AppFamilyInfo{}
	personCloud := &cloudpan.AppFamilyInfo{
		FamilyId: 0,
		RemarkName: "个人云",
		CreateTime: "-",
	}
	t = append(t, personCloud)
	t = append(t, familyResult.FamilyInfoList...)
	familyList := t
	builder := &strings.Builder{}
	tb := cmdtable.NewTable(builder)
	tb.SetColumnAlignment([]int{tablewriter.ALIGN_DEFAULT, tablewriter.ALIGN_RIGHT, tablewriter.ALIGN_CENTER, tablewriter.ALIGN_CENTER})
	tb.SetHeader([]string{"#", "family_id", "家庭云名", "创建日期"})

	for k, familyInfo := range familyList {
		tb.Append([]string{strconv.Itoa(k), strconv.FormatInt(familyInfo.FamilyId, 10), familyInfo.RemarkName, familyInfo.CreateTime})
	}
	tb.Render()
	return familyList, builder.String()
}
