# Straightcast

Straightcastは、同じPCの画面をH.264にしてローカルのRTSPへ出せます。WindowsではDXGIのddagrabで画面を取ります。エンコーダはNVENC、Quick Sync、AMFの順です。いずれも無ければlibx264を使います。LinuxとmacOSではテスト映像を出します。

ウィンドウ名を指定した場合は、そのウィンドウだけが対象です。取り込みはgfxcaptureで、フレームはGPU上のままエンコードします。

受け側のURLは2種類です。`rtspt://`はTCPで、`rtsp://`はUDPです。他のPCへ渡すときはTCPを使ってください。

配布物は実行ファイルとMediaMTXです。MediaMTXは受信した映像を再エンコードしません。

## 遅延

640×352のテストフレームを、スレッド2のlibx264で符号化しました。ローカルのRTSPをデコーダ1スレッドで読み戻した中央値は次のとおりです。プレイヤー側のバッファは含みません。

| fps | 中央値 | ffmpegのCPU |
| --- | --- | --- |
| 30 | 36ms | 約5% |
| 60 | 19ms | 約9% |

標準は30fpsです。60fpsにすると待ち時間は半分になります。そのぶんエンコーダの処理は2倍です。

デコーダがフレームを溜める読み方では約70msでした。ブラウザから配信していたときの同じ測り方は71msです。

Bフレームとlookaheadは無効です。NVENCはpreset p1、tune ull、delay 0を使います。VBVバッファは1フレーム分です。キーフレーム間隔は約1秒で、GOPはfpsと一致します。間隔を長くすると、後から接続したクライアントは映像を受け取れません。音声トラックは無しです。ハードウェアエンコードでは縮小しません。libx264のときだけ幅を1280に抑え、スレッド数は2です。配信プロセスの優先度は下げています。

## ダウンロード

バイナリは[Releases](https://github.com/hrdtbs/straightcast/releases)にあります。Windowsでは`straightcast_windows_amd64.zip`を展開し、`straightcast.exe`を起動してください。

ffmpegは同梱していません。[gyan.devのessentials](https://www.gyan.dev/ffmpeg/builds/)をPATHへ通すか、exeの隣の`bin/ffmpeg.exe`に置いてください。`ddagrab`と`h264_nvenc`が必要です。ウィンドウだけを送るときは`gfxcapture`も必要です。MediaMTXが無い場合、初回起動時に`bin/`へ取得します。

## 使い方

ソースから動かすにはGo 1.22以降とffmpegが必要です。

```sh
go run ./cmd/straightcast
```

初回起動でMediaMTX v1.21.1を`bin/`へ取得します。操作画面は[http://127.0.0.1:43123](http://127.0.0.1:43123)です。

画面に出たrtsptのURLをコピーしてください。ホスト欄には、このPCのアドレスが入ります。コピー後にタブを閉じても、配信は続きます。

LinuxとmacOSではデスクトップを取得できません。その場合はテスト映像を送ります。Windowsではウィンドウ名にタイトルの一部を入れると、そのウィンドウだけを送れます。名前が空のときはモニター全体です。ホスト欄が違うときは、このPCのアドレスへ変えてください。UDPは他のPCでは届かないことがあります。

## 開発

```sh
make test
make lint
make build-all
```

`make test`は`go test -race ./cmd/... ./internal/...`を実行します。`make lint`は[golangci-lint v1.64.8](https://github.com/golangci/golangci-lint)です。設定ファイルは`.golangci.yml`にあります。`make build-all`の対象はwindows/amd64とlinux/amd64です。darwinはamd64とarm64もコンパイルします。

GitHub Actionsの定義は`packaging/workflows/`です。`.github/workflows/`へ置くと、`main`へのpushでテストとlintが走ります。タグ`vX.Y.Z`を付けると、Releaseへバイナリを載せます。同じアーカイブを手元で作るには、次のコマンドを実行してください。

```sh
scripts/package.sh v0.1.0
```

## 計測

次のコマンドは、エンコードしてからRTSPを読み戻すまでの時間を測ります。

```sh
go run ./cmd/straightcast measure
```

## ビルド

```sh
go build -o straightcast ./cmd/straightcast
```

Windows向けは次のコマンドでビルドできます。

```sh
GOOS=windows GOARCH=amd64 go build -o straightcast.exe ./cmd/straightcast
```

`straightcast.exe`と同じ場所の`bin/`に`ffmpeg.exe`と`mediamtx.exe`を置くか、PATHを通してください。MediaMTXが無ければ、初回起動時に取得します。
