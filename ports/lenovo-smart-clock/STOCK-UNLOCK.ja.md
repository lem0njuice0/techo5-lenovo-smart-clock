# 純正データの取得とアンロック（初代 Smart Clock）

[English](STOCK-UNLOCK.en.md) | **日本語** · [導入ガイド](INSTALL.ja.md)

対象は初代 Lenovo Smart Clock CD-24501F です。[XDA の関連スレッド](https://xdaforums.com/t/lenovo-smart-clock-bootloader-avb-unlock-firmware-region-changer-kernel-source.4130295/)を参照した [kortregt 氏の解除記録](https://github.com/kortregt/lenovo-smart-clock-linux/blob/main/docs/01-unlocking.md)と[読み出し記録](https://github.com/kortregt/lenovo-smart-clock-linux/blob/main/docs/02-backup.md)をもとに、作業の流れを説明します。外部ツールや認証情報はこのリポジトリには含まれず、ファイルの安全性や、すべての純正バージョンとの適合は確認できていません。

解除や書き込みによって端末が初期化されたり、起動できなくなったりする可能性があります。復旧用の純正データを残したい場合は、**解除や書き込みより先に**取得してください。本プロジェクトでは完全な復元手順を実機で検証していません。

## 1. 純正データの読み出し

参照先では、[mtkclient](https://github.com/bkerler/mtkclient) を使い、時計を MediaTek BootROM モードで接続して eMMC の内容を読み出しています。BootROM 接続は fastboot とは別です。機種固有のボタン操作は参照先でも確定していません。使用する OS に合わせて mtkclient と USB ドライバーを準備し、[読み出し記録](https://github.com/kortregt/lenovo-smart-clock-linux/blob/main/docs/02-backup.md)の接続・権限に関する注意を確認してください。Windows から WSL を使う場合は、BootROM の USB 機器を WSL に接続する必要があります。

mtkclient で対象端末を認識できた場合、参照先に記載された読み出しコマンドは次のとおりです。`mtk` コマンドの実行方法はインストール方法によって異なります。出力先には十分な空き容量を用意し、コマンドの対象と保存先を確認してください。

```sh
mtk printgpt > gpt.txt
mtk rf full_user.bin
mtk r preloader boot1.bin --parttype boot1
mtk r preloader boot2.bin --parttype boot2
```

`full_user.bin` はユーザー領域全体で、参照先の端末では約 7.3 GiB です。`boot1.bin` と `boot2.bin` は別の eMMC ブート領域です。これらは機器固有の情報を含み得るため、GitHub などへ公開しないでください。読み出しの成功だけで復元できるとは限りません。保管する場合はサイズと SHA-256 を記録し、復旧方法を別途確認してください。

統合スクリプトは `full_user.bin` の GPT を検査し、ビルドに必要な boot・vendor・OEM を選択したスロットから抽出できます。**端末からの読み出し自体は自動化していません。** 純正の各パーティションをすでに持っている場合は、それらを個別に渡すこともできます。[導入ガイドの統合スクリプト](INSTALL.ja.md#統合スクリプト実験版)を参照してください。

## 2. fastboot モードに入る

データ通信に対応した USB ケーブルで時計と PC を接続したまま、時計の電源を抜きます。音量アップを押しながら電源を入れ、20〜30 秒ほど押し続けます。画面が暗いままでも、PC 側で次のコマンドを実行して認識を確認します。複数の端末が接続されている場合は、以後のコマンドで正しいシリアルを指定してください。

```sh
fastboot devices
fastboot -s DEVICE_SERIAL getvar product
fastboot -s DEVICE_SERIAL getvar unlocked
fastboot -s DEVICE_SERIAL getvar at-vboot-state
```

`DEVICE_SERIAL` は `fastboot devices` に表示された対象機の値に置き換えます。本機で確認した product は `mt8167s` ですが、同じ product 名だけで機種や互換性を確定することはできません。

## 3. ブートローダーの解除

参照先では、対象を確認したうえで次のコマンドを使っています。設定・データが消える可能性があります。時計や PC に確認画面が出た場合は、表示内容を読んで操作してください。

```sh
fastboot -s DEVICE_SERIAL flashing unlock
fastboot -s DEVICE_SERIAL getvar unlocked
```

成功の判定には `unlocked: yes` を確認します。参照先では、`at-vboot-state` 中の `bootloader-locked: 1` が解除後も残る例が報告されています。これだけを見て解除失敗とは判断しないでください。

## 4. Android Things の AVB 解除

ブートローダーと AVB は別のロックです。参照先によると、AVB の解除には当該製品用の認証情報が必要で、XDA に案内されていた `Cube-AVB-factory-Unlock.zip` 内の `cube_unlock_credentials_v2.zip` とツールを使う手順が紹介されています。**このプロジェクトは認証情報を配布・取得・検証していません。** 外部から入手したファイルの出所や利用条件、安全性は利用者が確認してください。

[解除記録](https://github.com/kortregt/lenovo-smart-clock-linux/blob/main/docs/01-unlocking.md)には、Windows 用ツールと、Linux／macOS 向け Python ツールの利用例が記載されています。Python 版の参照先は解除後に factory パーティションの persistent digest も消去すると説明しています。この操作は単なる状態確認ではありません。処理内容と、純正状態に戻したい場合の影響を理解できない場合は実行しないでください。利用可能な認証情報がない場合、この方法では先に進めません。

例えば、参照先の Python ツールと `uv` を別途用意し、`fastboot` を PATH に通した環境で、参照先が示しているコマンドは次のとおりです。作業ディレクトリにあるツールと認証情報を対象にする例で、コマンドは AVB 状態と persistent digest を変更します。複数の fastboot 端末は接続せず、対象を確認してください。

```sh
uv run --no-project --with pycryptodome python at_auth_unlock.py -v cube_unlock_credentials_v2.zip
```

解除後の確認には次を使います。

```sh
fastboot -s DEVICE_SERIAL getvar at-vboot-state
```

参照先では `avb-locked: 0` が AVB 解除の目安です。状態が確認できない場合は、イメージの書き込みに進まないでください。純正に戻す場合も、元のスロットや AVB の状態、変更した領域により手順が変わります。安易に再ロックせず、復旧手段を確認してください。
