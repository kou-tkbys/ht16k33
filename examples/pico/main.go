//go:build tinygo

package main

import (
	"machine"
	"time"

	"github.com/kou-tkbys/ht16k33"
)

func main() {
	// --- Raspberry Pi Pico用 I2Cバスのセットアップ ---
	i2c := machine.I2C0
	err := i2c.Configure(machine.I2CConfig{
		Frequency: 400 * machine.KHz,
		// PicoのI2C0で一般的に使われるピンを指定
		// SDA: GPIO4 (物理ピン6)
		// SCL: GPIO5 (物理ピン7)
		SDA: machine.GPIO4,
		SCL: machine.GPIO5,
	})
	if err != nil {
		println("could not configure I2C:", err.Error())
		return
	}

	// --- ドライバの初期化 ---
	// HT16K33のI2Cアドレス(通常は0x70)を指定
	display := ht16k33.New(i2c, 0x70)
	display.Configure()

	// フェードの速さを決める
	fadeDelay := 20 * time.Millisecond

	// --- デモ開始 ---
	println("HT16K33 Demo Start!")

	// --- ノンブロッキング版デモ ---
	// 状態を管理するための変数
	demoState := 0
	lastActionTime := time.Now()

	for {
		// UpdateFadeを毎回呼び出して、フェードアニメーションを動かす
		display.UpdateFade()

		// フェード中でなければ、次のデモステップに進む
		if !display.IsFading() && time.Since(lastActionTime) > 2*time.Second {
			switch demoState {
			case 0:
				println("1. Clearing all with fade...")
				display.ClearAll()
				display.StartFade(fadeDelay)
			case 1:
				println("2. Writing 'HELLO' to display 0...")
				display.WriteString(0, "HELLO")
				display.Display() // フェードなしで表示
			case 2:
				println("3. Writing 'WORLD' to display 1 with fade...")
				display.WriteString(1, "WORLD")
				display.StartFade(fadeDelay)
			case 3:
				println("4. Writing a long number with fade...")
				longNumber := "12345678.9012345."
				for i, char := range longNumber {
					// WriteStringはディスプレイをクリアしてしまうので、16
					// 桁全体に書き込むにはSetDigit16をループで使う。
					// ドットはWriteStringが自動で処理してくれるが、
					// SetDigit16の場合は自分で処理する必要があるため。
					// ここでは簡単にするためにドットは無視。`char` をその
					// まま渡すのがポイント。
					display.SetDigit16(i, char, false)
				}
				display.StartFade(fadeDelay)
			case 4:
				println("5. Clearing display 0 with fade...")
				display.ClearOnDisplay(0)
				display.StartFade(fadeDelay)
			case 5:
				println("6. Lighting up all segments (illumination mode)...")
				// これはブロッキング関数なので、完了するまでここで待機する
				display.LightUpAllFadeBlocking(fadeDelay)
			case 6:
				println("7. Demo Finished! Restarting...")
				demoState = -1 // 次のループで0になる
			}
			demoState++
			lastActionTime = time.Now()
		}

		// ここで他の処理ができる
		// 例えば、ボタン入力をチェックしたり、センサーの値を読んだり…
		time.Sleep(10 * time.Millisecond) // ループが速くなりすぎないように少し待つ
	}
}
