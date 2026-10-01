# Straightcast

同じ PC の中で、画面を RTSP に渡す配信です。ブラウザも Electron も使いません。Windows ではデスクトップのフレームを GPU のエンコード回路へ直接渡すので、CPU をほとんど取りません。

## なぜこの形か

短い経路は、同じマシン上の RTSP です。`rtspt://` は TCP、`rtsp://` は UDP です。TCP の方がジッタが小さく、遅延を優先するならこちらです。

ブラウザで画面を取ると、描画、エンコード、WebRTC、中継が重なります。Electron で包むと、その横にもう一つ Chromium が座ります。クラウドに置くと映像がネットを往復します。

Straightcast は小さな実行ファイルです。中継の MediaMTX は再エンコードせず、RTSP だけを開きます。

## 遅延

エンコードの待ちは、だいたい 1 フレームです。この Linux 環境で、テストフレームを libx264（超低遅延、スレッド 2）に通し、ローカルの RTSP を 1 スレッドのデコーダで読み戻した結果です。再生側のバッファは含んでいません。

| フレームレート | 中央値 | ffmpeg の CPU（640×352） |
| --- | --- | --- |
| 30 | 36ms | 約 5% |
| 60 | 19ms | 約 9% |

30fps が標準です。60fps は待ちが半分になりますが、エンコーダの仕事も倍です。

以前のブラウザ配信を、デコーダがフレームを溜める読み方で測ると中央値 71ms でした。同じ読み方だと、今回の再エンコードも約 70ms で、エンコードを挟んでもフレームは増えていません。上の 36ms は、読み側の余分なスレッド待ちを外した値です。

詰めてある点:

- B フレームなし。先読みなし。NVENC は出力遅延 0、超低遅延チューニング、最速プリセット。
- レート制御のバッファは 1 フレーム分。
- キーフレームは約 1 秒ごと。これより長くすると、あとから接続した受信側が映像を受け取れません。
- 音声なし。変換が遅延を足し、受信側のバッファも増えるためです。
- ハードウェアエンコードでは縮小も CPU へのコピーもしません。ソフトウェアに落ちたときだけ、幅 1280 まで、スレッド 2 に抑えます。
- 配信プロセスの優先度は他のプロセスより下です。

## ダウンロード

[Releases](https://github.com/hrdtbs/straightcast/releases) から OS ごとのアーカイブを取れます。Windows では `straightcast_windows_amd64.zip` を展開し、`straightcast.exe` を起動します。

ffmpeg は同梱していません。[gyan.dev の ffmpeg essentials](https://www.gyan.dev/ffmpeg/builds/) を PATH に通すか、exe の隣の `bin/ffmpeg.exe` に置いてください。`ddagrab` と `h264_nvenc` が入っているビルドが必要です。MediaMTX が無いときは、初回起動で公式リリースを `bin/` に取得します。

## 使い方

Go から動かす場合は Go 1.22 以降と ffmpeg が要ります。

```sh
go run ./cmd/straightcast
```

初回は MediaMTX v1.21.1 を `bin/` に取ります。制御画面は [http://127.0.0.1:43123](http://127.0.0.1:43123) です。

1. 表示された TCP の URL（`rtspt://127.0.0.1:8554/...`）をコピーする。UDP の `rtsp://` は受信側の揺れ吸収が入り、TCP より遅れやすいです。
2. URL をコピーしたあとは、このタブを閉じて構いません。配信は続きます。

Linux や macOS ではデスクトップを取れないので、テスト映像を送る。画面の取り込みは Windows の DXGI です。

別のマシンから見るときは、ホスト欄を相手から届くアドレスにします。UDP はネットワークを越えられないことがあり、届いても受信側のジッタの分だけ遅れます。

## 開発

```sh
make test      # go test -race ./cmd/... ./internal/...
make lint      # golangci-lint run
make build-all # Windows / Linux / macOS 向けにコンパイルできることを確認
```

lint は [golangci-lint v1.64.8](https://github.com/golangci/golangci-lint) を使います。設定は `.golangci.yml` です。

GitHub Actions の定義は `packaging/workflows/` にあり、`.github/workflows/` に置くと `main` への push でテストと lint が走り、タグ `vX.Y.Z` で Windows / Linux / macOS のバイナリを Release に載せます。手元で同じアーカイブを作るときは次です。

```sh
scripts/package.sh v0.1.0
```

## 計測

再生側は起動しません。中継だけの時間です。

```sh
go run ./cmd/straightcast measure
```

## ビルド

```sh
go build -o straightcast ./cmd/straightcast
```

Windows 向け:

```sh
GOOS=windows GOARCH=amd64 go build -o straightcast.exe ./cmd/straightcast
```

`straightcast.exe` と同じ場所の `bin/` に `ffmpeg.exe` と `mediamtx.exe` を置くか、PATH を通します。MediaMTX が無ければ初回起動時に取得します。
