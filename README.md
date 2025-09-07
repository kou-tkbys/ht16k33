# ht16k33

TinyGo向けの`HT16K33` I2C LEDドライバライブラリ

このドライバーは、Holtek社の`HT16K33`チップを使い、2つの8桁7セグメントディスプレイ（合計16桁）を制御するために作られている。

## 概要 (Overview)

`HT16K33`は、I2Cインターフェイスを介して、最大128個のLED（16x8）または8桁の7セグメントLEDを制御できる便利なIC。このライブラリは、`HT16K33`のROWピンとCOMピンを工夫して接続することで、1つのICで16桁の7セグメントディスプレイを駆動できるように設計した。

詳しい結線方法については、`ht16k33.go`の先頭にあるコメントを参照すること。

## 特徴 (Features)

*   2つの8桁ディスプレイ、または1つの16桁ディスプレイとしての制御
*   ディスプレイのON/OFF、輝度（明るさ）の調整
*   文字列や数値を簡単に表示 (`WriteString`)
*   ディスプレイ全体、または個別のディスプレイのクリア
*   ブロッキング/ノンブロッキングのフェードエフェクト
*   `machine.I2C` に対応

## 使い方 (Usage)

以下は、`WriteString`を使って2つのディスプレイに数値を表示する簡単な例：

```go
package main

import (
	"machine"
	"time"

	"github.com/kou-tkbys/ht16k33"
)

func main() {
	// I2Cバスを初期化
	machine.I2C0.Configure(machine.I2CConfig{})

	// ディスプレイドライバを作成して設定
	// アドレスは環境に合わせて変更すること
	display := ht16k33.New(machine.I2C0, 0x70)
	display.Configure()

	// それぞれの8桁ディスプレイに、違う文字列を書き込む
	display.WriteString(0, "3600")
	display.WriteString(1, "1800")

	// バッファの内容をディスプレイに反映させる
	display.Display()

	// このままプログラムが終わらないように待つ
	for {
		time.Sleep(time.Hour)
	}
}
```

## ライセンス (License)

MIT License
