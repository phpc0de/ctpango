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
	"github.com/phpc0de/ctpango/cmder/cmdtable"
	"github.com/phpc0de/ctpango/internal/config"
	"github.com/phpc0de/ctpango/internal/file/downloader"
	"github.com/phpc0de/ctpango/internal/functions/pandownload"
	"github.com/phpc0de/ctpango/internal/taskframework"
	"github.com/phpc0de/ctlibgo/converter"
	"github.com/phpc0de/ctpango/library/requester/transfer"
	"github.com/urfave/cli"
	"os"
	"path/filepath"
	"runtime"
)

type (
	//DownloadOptions 下载可选参数
	DownloadOptions struct {
		IsPrintStatus        bool
		IsExecutedPermission bool
		IsOverwrite          bool
		SaveTo               string
		Parallel             int
		Load                 int
		MaxRetry             int
		NoCheck              bool
		ShowProgress         bool
		FamilyId             int64
	}

	// LocateDownloadOption 获取下载链接可选参数
	LocateDownloadOption struct {
		FromPan bool
	}
)

var (
	// MaxDownloadRangeSize 文件片段最大值
	MaxDownloadRangeSize = 55 * converter.MB

	// DownloadCacheSize 默认每个线程下载缓存大小
	DownloadCacheSize = 64 * converter.KB
)

func CmdDownload() cli.Command {
	return cli.Command{
		Name:      "download",
		Aliases:   []string{"d"},
		Usage:     "下载文件/目录",
		UsageText: cmder.App().Name + " download <文件/目录路径1> <文件/目录2> <文件/目录3> ...",
		Description: `
	下载的文件默认保存到, 程序所在目录的 download/ 目录.
	通过 cloudpan189-go config set -savedir <savedir>, 自定义保存的目录.
	支持多个文件或目录下载.
	自动跳过下载重名的文件!

	示例:

	设置保存目录, 保存到 D:\Downloads
	注意区别反斜杠 "\" 和 斜杠 "/" !!!
	cloudpan189-go config set -savedir D:\\Downloads
	或者
	cloudpan189-go config set -savedir D:/Downloads

	下载 /我的资源/1.mp4
	cloudpan189-go d /我的资源/1.mp4

	下载 /我的资源 整个目录!!
	cloudpan189-go d /我的资源

    下载 /我的资源/1.mp4 并保存下载的文件到本地的 d:/panfile
	cloudpan189-go d --saveto d:/panfile /我的资源/1.mp4
`,
		Category: "天翼云盘",
		Before:   cmder.ReloadConfigFunc,
		Action: func(c *cli.Context) error {
			if c.NArg() == 0 {
				cli.ShowCommandHelp(c, c.Command.Name)
				return nil
			}

			// 处理saveTo
			var (
				saveTo string
			)
			if c.Bool("save") {
				saveTo = "."
			} else if c.String("saveto") != "" {
				saveTo = filepath.Clean(c.String("saveto"))
			}

			do := &DownloadOptions{
				IsPrintStatus:        c.Bool("status"),
				IsExecutedPermission: c.Bool("x"),
				IsOverwrite:          c.Bool("ow"),
				SaveTo:               saveTo,
				Parallel:             c.Int("p"),
				Load:                 c.Int("l"),
				MaxRetry:             c.Int("retry"),
				NoCheck:              c.Bool("nocheck"),
				ShowProgress:         !c.Bool("np"),
				FamilyId:             parseFamilyId(c),
			}

			RunDownload(c.Args(), do)
			return nil
		},
		Flags: []cli.Flag{
			cli.BoolFlag{
				Name:  "ow",
				Usage: "overwrite, 覆盖已存在的文件",
			},
			cli.BoolFlag{
				Name:  "status",
				Usage: "输出所有线程的工作状态",
			},
			cli.BoolFlag{
				Name:  "save",
				Usage: "将下载的文件直接保存到当前工作目录",
			},
			cli.StringFlag{
				Name:  "saveto",
				Usage: "将下载的文件直接保存到指定的目录",
			},
			cli.BoolFlag{
				Name:  "x",
				Usage: "为文件加上执行权限, (windows系统无效)",
			},
			cli.IntFlag{
				Name:  "p",
				Usage: "指定下载线程数",
			},
			cli.IntFlag{
				Name:  "l",
				Usage: "指定同时进行下载文件的数量",
			},
			cli.IntFlag{
				Name:  "retry",
				Usage: "下载失败最大重试次数",
				Value: pandownload.DefaultDownloadMaxRetry,
			},
			cli.BoolFlag{
				Name:  "nocheck",
				Usage: "下载文件完成后不校验文件",
			},
			cli.BoolFlag{
				Name:  "np",
				Usage: "no progress 不展示下载进度条",
			},
			cli.StringFlag{
				Name:  "familyId",
				Usage: "家庭云ID",
				Value: "",
			},
		},
	}
}

func downloadPrintFormat(load int) string {
	if load <= 1 {
		return pandownload.DefaultPrintFormat
	}
	return "\r[%s] ↓ %s/%s %s/s in %s, left %s ..."
}

// RunDownload 执行下载网盘内文件
func RunDownload(paths []string, options *DownloadOptions) {
	if options == nil {
		options = &DownloadOptions{}
	}

	if options.Load <= 0 {
		options.Load = config.Config.MaxDownloadLoad
	}

	if options.MaxRetry < 0 {
		options.MaxRetry = pandownload.DefaultDownloadMaxRetry
	}

	if runtime.GOOS == "windows" {
		// windows下不加执行权限
		options.IsExecutedPermission = false
	}

	// 设置下载配置
	cfg := &downloader.Config{
		Mode:                       transfer.RangeGenMode_BlockSize,
		CacheSize:                  config.Config.CacheSize,
		BlockSize:                  MaxDownloadRangeSize,
		MaxRate:                    config.Config.MaxDownloadRate,
		InstanceStateStorageFormat: downloader.InstanceStateStorageFormatJSON,
		ShowProgress: options.ShowProgress,
	}
	if cfg.CacheSize == 0 {
		cfg.CacheSize = int(DownloadCacheSize)
	}

	// 设置下载最大并发量
	if options.Parallel < 1 {
		options.Parallel = config.Config.MaxDownloadParallel
	}

	paths, err := matchPathByShellPattern(options.FamilyId, paths...)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Print("\n")
	fmt.Printf("[0] 提示: 当前下载最大并发量为: %d, 下载缓存为: %d\n", options.Parallel, cfg.CacheSize)

	var (
		panClient = GetActivePanClient()
		loadCount = 0
	)

	// 预测要下载的文件数量
	for k := range paths {
		// 使用递归获取文件的方法计算路径包含的文件的总数量
		panClient.AppFilesDirectoriesRecurseList(options.FamilyId, paths[k], func(depth int, _ string, fd *cloudpan.AppFileEntity, apiError *apierror.ApiError) bool {
			if apiError != nil {
				panCommandVerbose.Warnf("%s\n", apiError)
				return true
			}

			// 忽略统计文件夹数量
			if !fd.IsFolder {
				loadCount++
				if loadCount >= options.Load { // 文件的总数量超过指定的指定数量，则不再进行下层的递归查找文件
					return false
				}
			}
			return true
		})

		if loadCount >= options.Load {
			break
		}
	}

	// 修改Load, 设置MaxParallel
	if loadCount > 0 {
		options.Load = loadCount
		// 取平均值
		cfg.MaxParallel = config.AverageParallel(options.Parallel, loadCount)
	} else {
		cfg.MaxParallel = options.Parallel
	}

	var (
		executor = taskframework.TaskExecutor{
			IsFailedDeque: true, // 统计失败的列表
		}
		statistic = &pandownload.DownloadStatistic{}
	)
	// 处理队列
	for k := range paths {
		newCfg := *cfg
		unit := pandownload.DownloadTaskUnit{
			Cfg:                  &newCfg, // 复制一份新的cfg
			PanClient:            panClient,
			VerbosePrinter:       panCommandVerbose,
			PrintFormat:          downloadPrintFormat(options.Load),
			ParentTaskExecutor:   &executor,
			DownloadStatistic:    statistic,
			IsPrintStatus:        options.IsPrintStatus,
			IsExecutedPermission: options.IsExecutedPermission,
			IsOverwrite:          options.IsOverwrite,
			NoCheck:              options.NoCheck,
			FilePanPath:          paths[k],
			FamilyId:             options.FamilyId,
		}

		// 设置储存的路径
		if options.SaveTo != "" {
			unit.OriginSaveRootPath = options.SaveTo
			unit.SavePath = filepath.Join(options.SaveTo, filepath.Base(paths[k]))
		} else {
			// 使用默认的保存路径
			unit.OriginSaveRootPath = GetActiveUser().GetSavePath("")
			unit.SavePath = GetActiveUser().GetSavePath(paths[k])
		}
		info := executor.Append(&unit, options.MaxRetry)
		fmt.Printf("[%s] 加入下载队列: %s\n", info.Id(), paths[k])
	}

	// 开始计时
	statistic.StartTimer()

	// 开始执行
	executor.Execute()

	fmt.Printf("\n下载结束, 时间: %s, 数据总量: %s\n", statistic.Elapsed()/1e6*1e6, converter.ConvertFileSize(statistic.TotalSize()))

	// 输出失败的文件列表
	failedList := executor.FailedDeque()
	if failedList.Size() != 0 {
		fmt.Printf("以下文件下载失败: \n")
		tb := cmdtable.NewTable(os.Stdout)
		for e := failedList.Shift(); e != nil; e = failedList.Shift() {
			item := e.(*taskframework.TaskInfoItem)
			tb.Append([]string{item.Info.Id(), item.Unit.(*pandownload.DownloadTaskUnit).FilePanPath})
		}
		tb.Render()
	}
}
