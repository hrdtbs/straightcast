# Straightcast

実行ファイルは[Releases](https://github.com/hrdtbs/straightcast/releases)にあります。Windowsは`straightcast_windows_amd64.zip`を展開し、`straightcast.exe`を起動してください。

ffmpegは同梱していません。[gyan.devのessentials](https://www.gyan.dev/ffmpeg/builds/)をPATHへ通すか、exeの隣の`bin/ffmpeg.exe`に置いてください。

## 開発

```sh
make test
make lint
make build-all
scripts/package.sh vX.Y.Z
go build -o straightcast ./cmd/straightcast
```
