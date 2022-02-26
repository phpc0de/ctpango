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
	"github.com/phpc0de/ctapi/cloudpan"
	"github.com/phpc0de/ctapi/cloudpan/apierror"
	"github.com/phpc0de/ctpango/cmder"
	"github.com/phpc0de/ctpango/internal/config"
	"github.com/urfave/cli"
	"path"
	"strings"
)

func CmdMkdir() cli.Command {
	return cli.Command{
		Name:      "mkdir",
		Usage:     "创建目录",
		UsageText: cmder.App().Name + " mkdir <目录>",
		Category:  "天翼云盘",
		Before:    cmder.ReloadConfigFunc,
		Action: func(c *cli.Context) error {
			if c.NArg() == 0 {
				cli.ShowCommandHelp(c, c.Command.Name)
				return nil
			}
			if config.Config.ActiveUser() == nil {
				fmt.Println("未登录账号")
				return nil
			}
			RunMkdir(parseFamilyId(c), c.Args().Get(0))
			return nil
		},
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "familyId",
				Usage: "家庭云ID",
				Value: "",
			},
		},
	}
}

func RunMkdir(familyId int64, name string) {
	activeUser := GetActiveUser()
	fullpath := activeUser.PathJoin(familyId, name)
	pathSlice := strings.Split(fullpath, "/")
	rs := &cloudpan.AppMkdirResult{}
	err := apierror.NewFailedApiError("")

	var cWorkDir = activeUser.Workdir
	var cFileId = activeUser.WorkdirFileEntity.FileId
	if IsFamilyCloud(familyId) {
		cWorkDir = activeUser.FamilyWorkdir
		cFileId = activeUser.FamilyWorkdirFileEntity.FileId
	}
	if path.Dir(fullpath) == cWorkDir {
		rs, err = activeUser.PanClient().AppMkdirRecursive(familyId, cFileId, path.Clean(path.Dir(fullpath)), len(pathSlice) - 1, pathSlice)
	} else {
		rs, err = activeUser.PanClient().AppMkdirRecursive(familyId,"", "", 0, pathSlice)
	}

	if err != nil {
		fmt.Println("创建文件夹失败：" + err.Error())
		return
	}

	if rs.FileId != "" {
		fmt.Println("创建文件夹成功: ", fullpath)
	} else {
		fmt.Println("创建文件夹失败: ", fullpath)
	}
}