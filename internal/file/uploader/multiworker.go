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
package uploader

import (
	"context"
	"github.com/phpc0de/ctpango/internal/waitgroup"
	"github.com/oleiade/lane"
	"os"
	"strconv"
)

type (
	worker struct {
		id         int
		partOffset int64
		splitUnit  SplitUnit
		uploadDone   bool
	}

	workerList []*worker
)

func (werl *workerList) Readed() int64 {
	var readed int64
	for _, wer := range *werl {
		readed += wer.splitUnit.Readed()
	}
	return readed
}

func (muer *MultiUploader) upload() (uperr error) {
	err := muer.multiUpload.Precreate()
	if err != nil {
		return err
	}

	var (
		uploadDeque = lane.NewDeque()
	)

	// 加入队列
	for _, wer := range muer.workers {
		if !wer.uploadDone {
			uploadDeque.Append(wer)
		}
	}

	for {
		wg := waitgroup.NewWaitGroup(muer.config.Parallel)
		for {
			e := uploadDeque.Shift()
			if e == nil { // 任务为空
				break
			}

			wer := e.(*worker)
			wg.AddDelta()
			go func() {
				defer wg.Done()

				var (
					ctx, cancel = context.WithCancel(context.Background())
					doneChan    = make(chan struct{})
					uploadDone  bool
					terr        error
				)
				go func() {
					if !wer.uploadDone {
						uploaderVerbose.Info("begin to upload part: " + strconv.Itoa(wer.id))
						uploadDone, terr = muer.multiUpload.UploadFile(ctx, int(wer.id), wer.partOffset, wer.splitUnit.Range().End, wer.splitUnit)
					} else {
						uploadDone = true
					}
					close(doneChan)
				}()
				select {
				case <-muer.canceled:
					cancel()
					return
				case <-doneChan:
					// continue
					uploaderVerbose.Info("multiUpload worker upload file done")
				}
				cancel()
				if terr != nil {
					if me, ok := terr.(*MultiError); ok {
						if me.Terminated { // 终止
							muer.closeCanceledOnce.Do(func() { // 只关闭一次
								close(muer.canceled)
							})
							uperr = me.Err
							return
						}
					}

					uploaderVerbose.Warnf("upload err: %s, id: %d\n", terr, wer.id)
					wer.splitUnit.Seek(0, os.SEEK_SET)
					uploadDeque.Append(wer)
					return
				}
				wer.uploadDone = uploadDone

				// 通知更新
				if muer.updateInstanceStateChan != nil && len(muer.updateInstanceStateChan) < cap(muer.updateInstanceStateChan) {
					muer.updateInstanceStateChan <- struct{}{}
				}
			}()
		}
		wg.Wait()

		// 没有任务了
		if uploadDeque.Size() == 0 {
			break
		}
	}

	select {
	case <-muer.canceled:
		if uperr != nil {
			return uperr
		}
		return context.Canceled
	default:
	}

	// upload file commit
	// 检测是否全部分片上传成功
	allSuccess := true
	for _, wer := range muer.workers {
		allSuccess = allSuccess && wer.uploadDone
	}
	if allSuccess {
		e := muer.multiUpload.CommitFile()
		if e != nil {
			uploaderVerbose.Warn("upload file commit failed: " + e.Error())
			return e
		}
	} else {
		uploaderVerbose.Warn("upload file not all success: " + muer.uploadFileId)
	}

	return
}
