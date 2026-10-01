# Straightcast

同じ PC で画面を H.264 にして、ローカルの RTSP に出す。Windows は DXGI（ddagrab）で取って NVENC、Quick Sync、AMF の順に試す。どれも無ければ libx264。Linux と macOS はテスト映像。

`rtspt://` は TCP、`rtsp://` は UDP。同じ PC で受けるなら TCP。

実行ファイルと MediaMTX だけ。MediaMTX は再エンコードしない。

## 遅延

640×352 のテストフレームを libx264（スレッド 2）に通し、ローカルの RTSP をデコーダ 1 スレッドで読み戻した中央値。プレイヤーのバッファは入っていない。

| fps | 中央値 | ffmpeg の CPU |
| --- | --- | --- |
| 30 | 36ms | 約 5% |
| 60 | 19ms | 約 9% |

30fps が標準。60fps は待ちが半分で、エンコーダの仕事は倍。

デコーダがフレームを溜める読み方だと約 70ms。ブラウザから配っていたときの同じ測り方は 71ms。

- B フレームなし、lookahead なし。NVENC は preset p1、tune ull、delay 0。
- VBV は 1 フレーム。
- キーフレームは約 1 秒（GOP = fps）。長くすると、後から繋いだクライアントが映像を受け取れない。
- 音声なし。
- ハードウェアエンコードでは縮小しない。libx264 のときだけ幅 1280、スレッド 2。
- 配信プロセスの優先度は下げる。

## ダウンロード

[Releases](https://github.com/hrdtbs/straightcast/releases) にある。Windows は `straightcast_windows_amd64.zip` を展開して `straightcast.exe`。

ffmpeg は入っていない。[gyan.dev の essentials](https://www.gyan.dev/ffmpeg/builds/) を PATH に通すか、exe の隣の `bin/ffmpeg.exe` に置く。`ddagrab` と `h264_nvenc` が要る。MediaMTX が無ければ初回起動で `bin/` に取る。

## 使い方

Go 1.22 以降と ffmpeg。

```sh
go run ./cmd/straightcast
```

初回に MediaMTX v1.21.1 を `bin/` に取る。画面は [http://127.0.0.1:43123](http://127.0.0.1:43123)。

`rtspt://127.0.0.1:8554/...` をコピーしたあとはタブを閉じていい。配信は続く。

Linux と macOS はデスクトップを取れないのでテスト映像。別のマシンから見るときはホスト欄を変える。UDP は届かないことがある。

## 開発

```sh
make test      # go test -race ./cmd/... ./internal/...
make lint      # golangci-lint run
make build-all # windows/amd64, linux/amd64, darwin/amd64, darwin/arm64
```

lint は [golangci-lint v1.64.8](https://github.com/golangci/golangci-lint)。設定は `.golangci.yml`。

Actions の定義は `packaging/workflows/`。`.github/workflows/` に置くと、`main` への push でテストと lint、タグ `vX.Y.Z` で Release にバイナリを載せる。手元で同じアーカイブを作るとき:

```sh
scripts/package.sh v0.1.0
```

## 計測

エンコードして RTSP を読み戻すまでの時間。

```sh
go run ./cmd/straightcast measure
```

## ビルド

```sh
go build -o straightcast ./cmd/straightcast
```

Windows:

```sh
GOOS=windows GOARCH=amd64 go build -o straightcast.exe ./cmd/straightcast
```

`straightcast.exe` と同じ場所の `bin/` に `ffmpeg.exe` と `mediamtx.exe` を置くか、PATH を通す。MediaMTX が無ければ初回起動で取る。
