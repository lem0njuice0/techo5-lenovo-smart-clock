# 初代 Lenovo Smart Clock：実験版の導入手順

[日本語 README](README.ja.md) / [English README](README.md)

このガイドでは、CD-24501F（`cube`、2019 年版）で実際に使った開発手順を紹介します。**一般向けの完成した手順ではありません。** 純正データの取得、ブートローダーの解除、カーネルの再ビルド、起動成功の登録、完全な復旧については、初期状態から作業できる手順がまだ整っていません。必要なファイルのうち、提供できないものは以下に記載しています。別の Lenovo Smart Clock や Echo には使用しないでください。

2 台で画面、タッチ、Wi-Fi などを検証しましたが、全機能・純正ファームウェアの全バージョン・復旧手段の検証は完了していません。試験用イメージの起動後も、データ保持や更新機能が自動的に有効になるわけではありません。

## 最初に：どこまでできる？

| 工程 | 現在の状態 | 案内 |
| --- | --- | --- |
| 純正データを用意・機器を解除 | **未完成**：各自で用意・解除する必要あり | [1. 用意するもの](#1-用意するもの) |
| 純正データから必要なファイルを抽出 | スクリプトあり | [2. 純正データから必要な入力を抽出する](#2-純正データから必要な入力を抽出する) |
| カーネルとユーザー空間をビルド | カーネルのパッチ・設定の入手と再現は未完成 | [3. ビルド用入力を用意する](#3-ビルド用入力を用意する) |
| イメージ作成・検査 | 準備済みの入力があれば実験用ツールを使用可能 | [4. ルートイメージとブートイメージを作成する](#4-ルートイメージとブートイメージを作成する) |
| 試験起動・書き込み | 非アクティブ側の内容を上書き。成功登録の一般向け手順は未完成 | [5. fastboot での試験と導入](#5-fastboot-での試験と導入) |

**このガイドだけでは、初期状態の機器への導入を完了できません。** 必要なファイルや解除・復旧の手段を用意できない場合は、書き込みに進まず、手順が整うまでお待ちください。

## 1. 用意するもの

- 対象機器、電源、データ通信に対応した USB ケーブル、Python 3、Android platform-tools の `fastboot`。Linux でイメージを作る場合は root 権限、`debugfs`、`mke2fs`、`qemu-arm` の binfmt 登録と Alpine のビルドに必要なツールも必要です。APK の音声設定抽出には JADX が必要です。
- 所有者が用意した対象機器の純正 `vendor`、`oem` の raw ext4 イメージまたは展開済みディレクトリ、および対象機器に対応する純正 boot イメージ。取得や復元の一般向け手順はまだありません。重要なデータと復旧方法は事前に確認してください。
- ブートローダーと AVB が解除済みの機器。解除方法や必要な資格情報はこのリポジトリでは提供していません。解除で初期化される場合があります。
- Lenovo の 2019 年公開カーネルソースに必要なパッチを適用したもの、対応する純正設定、ARM64 クロスコンパイラ。カーネルのパッチ一式や完全な取得・適用手順は未整備です。
- ルートファイルシステム用の Alpine `apk.static`、`alpine-minirootfs-3.24.2-armv7.tar.gz`、ウェイクワードモデル（`.tflite` と `.json`）。`ports/lenovo-smart-clock/inputs/`、または `CUBE_INPUTS` で指定したディレクトリに置きます。これらは自動収集されません。

以下のコマンドは Linux シェルで、リポジトリのルートにいる前提です。`/private/cube` と各入力パスは例です。機器のバックアップや抽出物を Git 管理対象に入れず、それぞれアクセス権を制限した場所に保管してください。書き込み先の新規ディレクトリは各ツールが作成します。

## 2. 純正データから必要な入力を抽出する

まず次の変数を実際のファイルに合わせて指定します。元のファイルは編集しません。raw ext4 イメージに Android sparse 形式などをそのまま渡すことはできません。

```sh
PORT=ports/lenovo-smart-clock
PRIVATE=/private/cube
VENDOR_IMG=/private/source/vendor.img
OEM_IMG=/private/source/oem.img
mkdir -p "$PRIVATE"
python3 "$PORT/tools/extract_stock_firmware.py" "$VENDOR_IMG" "$PRIVATE/firmware"
python3 "$PORT/tools/extract_oem_apk.py" "$OEM_IMG" "$PRIVATE/oem-apk"
```

`firmware` には Wi-Fi／Bluetooth 関連ファイルとハッシュを記録した `manifest.json` ができます。タッチ用ファームウェアは抽出されません。次に `oem-apk` に出力された APK の実際の名前を確認し、以下の `OEM_APK` をそのファイルに置き換えます。異なる純正バージョンでは必要なファイルや構造が一致しないことがあります。

```sh
OEM_APK=/private/cube/oem-apk/実際のAPKファイル名.apk
python3 "$PORT/tools/extract_audio_apk.py" "$OEM_APK" "$PRIVATE/audio"
```

音声設定は JADX で 2019 年版 APK の `Lenovo.java` を解析して作ります。失敗した場合は別機種や別バージョンのデータを無理に流用しないでください。詳細はそれぞれ `--help` で確認できます。

## 3. ビルド用入力を用意する

Windows の PowerShell と Go がある場合、`ports/lenovo-smart-clock/tools/build-cube-userspace.ps1` は実験用デーモン、Bluetooth ブリッジ、診断用バイナリ、カーネル内 init をビルドします。標準の診断バイナリに実験的 OTA 機能は含まれません。

Linux では同等のビルド例は次のとおりです。Go のバージョンなど各モジュールの要求を満たす必要があります。

```sh
mkdir -p bin "$PORT/build"
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go -C echod build -tags cube -o "$(pwd)/bin/echod-cube-arm" ./cmd/echod
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -o bin/btbridge-cube-arm ./cmd/btbridge
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go -C "$PORT/serial" build -tags cube_slot,cube_shared -o "$(pwd)/$PORT/build/cube-serial-arm" .
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go -C "$PORT/rescue" build -tags cube_boot -o "$(pwd)/$PORT/build/cube-boot-init-aarch64" .
```

パッチ適用済みのカーネルソースと実機に対応する設定が用意できた場合に限り、次のようにビルドを開始できます。`CUBE_KERNEL_BUILD` は存在しないパスを指定し、すべて実環境の絶対パスに置き換えます。パスに空白は使えません。

```sh
export CUBE_KERNEL_SRC=/private/source/patched-kernel
export CUBE_STOCK_CONFIG=/private/source/stock.config
export CUBE_RUNTIME_INIT="$(pwd)/$PORT/build/cube-boot-init-aarch64"
export CUBE_KERNEL_BUILD="$PRIVATE/kernel-build"
export CROSS_COMPILE=aarch64-linux-gnu-
bash "$PORT/tools/build_cube_kernel.sh"
export CUBE_KERNEL_OUT="$CUBE_KERNEL_BUILD/out"
```

このスクリプトはソースへのパッチ適用を行いません。公開版のツールだけからカーネルが再現できることもまだ確認できていません。

## 4. ルートイメージとブートイメージを作成する

必要な Alpine のファイルなどを `CUBE_INPUTS` に揃え、出力先にはまだ存在しないディレクトリを指定します。`CUBE_TEST_AUDIO=1` は 2019 年版の音声初期化を有効にする実験用設定です。別途用意する受信機用のプログラムがなければ、AirPlay などの機能は利用できません。

```sh
export CUBE_FIRMWARE="$PRIVATE/firmware"
export CUBE_AUDIO="$PRIVATE/audio"
export CUBE_INPUTS=/private/cube-inputs
export CUBE_OUTPUT="$PRIVATE/rootfs-output"
export CUBE_TEST_AUDIO=1
export CUBE_BLUETOOTH=1
sudo --preserve-env=CUBE_FIRMWARE,CUBE_AUDIO,CUBE_INPUTS,CUBE_OUTPUT,CUBE_KERNEL_OUT,CUBE_TEST_AUDIO,CUBE_BLUETOOTH bash "$PORT/tools/build-private-rootfs.sh"
```

出力される `system_b-experimental-DO-NOT-FLASH.img` は非公開の実験用イメージです。ファイル名だけでスロットとの互換性や安全性は判断できません。既定では設定の共有や OTA は有効になりません。日本語フォント取得や追加ヘルパーの条件は英語版 README を参照してください。

次は、対応する純正 donor boot、ビルドしたカーネルと DTB を入力に、**A を対象とする場合の例**です。B が正常な現在のスロットである場合だけ、この A の例が当てはまります。B 用は `--experimental-system-b` と `--expect-system-b` を使用し、ほかの手順でも対象を B に合わせます。

```sh
DONOR_BOOT=/private/source/boot.img
KERNEL_GZ="$CUBE_KERNEL_OUT/arch/arm64/boot/Image.gz"
DTB="$CUBE_KERNEL_OUT/arch/arm64/boot/dts/mediatek/mt8167s_ref.dtb"
BOOT="$PRIVATE/boot-a.img"
python3 "$PORT/tools/make_diagnostic_boot.py" --experimental-system-a "$DONOR_BOOT" "$KERNEL_GZ" "$DTB" "$BOOT"
python3 "$PORT/tools/verify_diagnostic_boot.py" --expect-system-a "$DONOR_BOOT" "$BOOT" "$KERNEL_GZ" "$DTB"
SYSTEM="$CUBE_OUTPUT/system_b-experimental-DO-NOT-FLASH.img"
python3 "$PORT/tools/verify_private_rootfs.py" --serial "$PORT/build/cube-serial-arm" --expect-test-audio --expect-test-bluetooth --expect-japanese-font --audio "$CUBE_AUDIO" "$SYSTEM" bin/echod-cube-arm "$CUBE_FIRMWARE" > "$PRIVATE/root-report.json"
```

すべての検査が成功しても実機での起動・復旧は保証されません。作成するイメージには独自の純正由来データが含まれます。再配布条件を確認せずに公開しないでください。

## 5. fastboot での試験と導入

この節はスクリプトの使い方を示す開発者向け例です。試験では**選択した非アクティブ側の `system` が実際に上書きされます**。そのスロットの純正システムを残したい場合は実行できません。使用する PC と機器で復旧経路が確立していることを確認してください。

機器の電源を抜き、USB を接続した状態で音量アップを押しながら電源を入れ、fastboot で認識されることを確認します。`SERIAL` は `fastboot devices` に表示された対象機器の値に、`BOOT_SHA256` は検証した boot の SHA-256 に置き換えます。別の機器のシリアルを指定しないでください。

```sh
fastboot devices
SERIAL=表示された対象機器のシリアル
fastboot -s "$SERIAL" getvar current-slot
fastboot -s "$SERIAL" getvar unlocked
fastboot -s "$SERIAL" getvar at-vboot-state
BOOT_SHA256=$(sha256sum "$BOOT" | cut -d ' ' -f 1)
python3 "$PORT/tools/install_cube_fastboot.py" --serial "$SERIAL" --slot a --boot "$BOOT" --boot-sha256 "$BOOT_SHA256" --system "$SYSTEM" --root-report "$PRIVATE/root-report.json" --action inspect
```

`inspect` はファイルと接続状態を検査します。A の例では現在 B が成功済みかつ起動可能で、A が非アクティブ、boot と rootfs が対象パーティションに適合し、ブートローダーと AVB が解除されている必要があります。エラーが出た場合、条件を無理に回避しないでください。

### 試験起動：A の system を上書きする

検査が通り、対象の A を上書きすることを理解している場合、上記コマンドの最後を `--action trial` に変えます。`system_a` を書き込み、その boot イメージを一時起動します。画面・タッチ・Wi-Fi・音声と必要な復旧経路を確認してください。試験起動の root と、ブートローダーが記録する現在のスロットは異なることがあります。

### 通常起動：A の boot を書き込み A を選ぶ

同じイメージ・同じ機器で試験に成功したら、fastboot に戻し、同じコマンドの最後を `--action install --trial-confirmed` に変えます。`boot_a` を書き込み A を選択します。その後 `fastboot -s "$SERIAL" reboot` で通常起動し、もう一度各機能を確認します。

**この操作だけでは A を起動成功として登録しません。** 成功登録前は試行回数が減り、条件によって元のスロットに戻ります。安全な成功登録・ロールバック・共通データ領域の移行を含む一般向け手順は未完成です。両スロット間での設定保持や自動更新も、この手順だけでは有効になりません。
