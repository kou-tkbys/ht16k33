// Official Datasheet (HT16K33/HT16K33A):
// https://www.holtek.com/webapi/116711/HT16K33Av102.pdf
//
// This driver does not cover all functions of the HT16K33.
// It implements the necessary features to control two 8-digit,
// common-cathode 7-segment displays using a single HT16K33 IC.
//
// The driver provides two ways to interact with the displays:
//  1. As two independent 8-digit displays (`SetDigitOnDisplay`, `WriteString`).
//  2. As a single, continuous 16-digit display (`SetDigit16`).
//
// Wiring Overview:
// This driver utilizes a clever multiplexing technique to drive 16 digits
// with an IC that normally supports 8. It treats the two 8-digit displays
// as a single 16x8 matrix.
//
//		+-----------------------------------------------------------------+
//		|                            HT16K33 IC                           |
//		+-----------------------------------------------------------------+
//		|        ROW 0-7         |        ROW 8-15        |    COM 0-7    |
//		+-----------|------------+------------|-----------+-------|-------+
//		            |                         |                   |
//		            |                         |                   +------> To Cathodes of Digits 1-8
//		            |                         |                            (Shared by both displays)
//		+-----------v------------+ +----------v----------+
//		|   Segments (Anodes)    | |   Segments (Anodes)   |
//		|     for Display A      | |     for Display B     |
//		| (a,b,c,d,e,f,g,dp)     | | (a,b,c,d,e,f,g,dp)    |
//		+------------------------+ +-----------------------+
//
//	  - COM0-COM7 (Digit Selectors):
//	    Each COM pin is connected to the common cathode of the corresponding digit
//	    on *both* displays. For example, COM0 connects to the cathode of digit 1
//	    on Display A AND the cathode of digit 1 on Display B.
//
// - ROW0-ROW15 (Segment Drivers):
//   - ROW0-ROW7 are connected to the segment anodes (a, b, c, d, e, f, g, dp)
//     of Display A.
//   - ROW8-ROW15 are connected to the segment anodes (a, b, c, d, e, f, g, dp)
//     of Display B.
package ht16k33

import "time"

const (
	// Commands for HT16K33
	ht16k33TurnOnOscillator = 0x21
	ht16k33TurnOnDisplay    = 0x81
	ht16k33SetBrightness    = 0xE0

	// MaxDigitsPerDisplay is the number of 7-segment digits per display unit.
	MaxDigitsPerDisplay = 8
	// NumDisplays is the number of display units driven by one HT16K33.
	NumDisplays = 2
)

// 7-segment display number patterns (g-f-e-d-c-b-a) plus a blank pattern.
var font = [11]byte{
	0b00111111, // 0
	0b00000110, // 1
	0b01011011, // 2
	0b01001111, // 3
	0b01100110, // 4
	0b01101101, // 5
	0b01111101, // 6
	0b00000111, // 7
	0b01111111, // 8
	0b01101111, // 9
	0b00000000, // 10 (blank)
}

const blankPatternIndex = 10

// I2CBus is an interface that abstracts the I2C Tx method we need.
//
// I2CBusは、必要とするI2CのTxメソッドを抽象化するインターフェース
type I2CBus interface {
	Tx(addr uint16, w, r []byte) error
}

// Device represents an HT16K33 device.
//
// Deviceは、HT16K33デバイス
type Device struct {
	bus     I2CBus
	Address uint8
	// Display RAM buffer for the HT16K33 (16x8 bits).
	// HT16K33の表示用RAMバッファ(16x8ビット)
	buffer [16]byte
	// currentBrightness holds the current brightness level (0-15).
	// currentBrightnessは、現在の明るさのレベル(0-15)を保持する。
	currentBrightness uint8
}

// New creates a new Device instance.
//
// Newは、新しいDeviceインスタンスを作る
func New(bus I2CBus, address uint8) Device {
	return Device{
		bus:               bus,
		Address:           address,
		currentBrightness: 15, // Default to max brightness
	}
}

// Configure initializes the HT16K33 device.
// It turns on the oscillator and the display, and sets the brightness to
// maximum.
//
// Configureは、HT16K33デバイスを初期化する
// オシレーターとディスプレイをオンにし、明るさを最大に設定する。
func (d *Device) Configure() {
	d.bus.Tx(uint16(d.Address), []byte{ht16k33TurnOnOscillator}, nil)
	d.bus.Tx(uint16(d.Address), []byte{ht16k33TurnOnDisplay}, nil)
	// Set to maximum brightness for now
	d.SetBrightness(15)
}

// ClearAll clears the entire display buffer, turning off all segments on
// both displays.
//
// ClearAllは、表示バッファ全体をクリアし、両方のディスプレイの全セグメン
// トを消灯させる。
func (d *Device) ClearAll() {
	for i := range d.buffer {
		d.buffer[i] = 0
	}
}

// SetDigitOnDisplay sets a single digit on one of the two displays.
// It first clears the previous content at that position for that display.
//
// SetDigitOnDisplayは、2つのディスプレイのいずれかに1桁を設定する。
//
// display: 0 for the first display (A), 1 for the second (B).
// position: 0-7, the digit position.
// num: 0-9, or use a value >= 10 for a blank digit.
// dot: true to light up the decimal point.
func (d *Device) SetDigitOnDisplay(display int, position int, num byte, dot bool) {
	if display < 0 || display >= NumDisplays || position < 0 || position >= MaxDigitsPerDisplay {
		return
	}

	var pattern byte
	if num < 10 {
		pattern = font[num]
	} else {
		pattern = font[blankPatternIndex]
	}

	rowOffset := display * MaxDigitsPerDisplay // 0 for display A, 8 for display B
	mask := ^byte(1 << position)               // Mask to clear the bit for the current position

	// Clear the bits for this digit position first
	for i := 0; i < MaxDigitsPerDisplay; i++ { // 7 segments + 1 dot
		d.buffer[rowOffset+i] &= mask
	}

	// Set the new segment bits (a-g -> ROW0-6 for display 0, ROW8-14 for display 1)
	for seg := 0; seg < 7; seg++ {
		if (pattern>>seg)&1 == 1 {
			d.buffer[rowOffset+seg] |= (1 << position)
		}
	}

	// Set the new dot bit (dp -> ROW7 for display 0, ROW15 for display 1)
	if dot {
		dotRow := rowOffset + 7
		d.buffer[dotRow] |= (1 << position)
	}
}

// SetDigit16 treats the two 8-digit displays as a single 16-digit display.
// It sets a single digit at a position from 0 to 15.
//
// SetDigit16は、2つの8桁ディスプレイを1つの16桁ディスプレイとして扱う。
// 0から15までの位置に1桁を設定する。
//
// position: 0-15, the digit position across both displays.
// num: 0-9, or use a value >= 10 for a blank digit.
// dot: true to light up the decimal point.
func (d *Device) SetDigit16(position int, num byte, dot bool) {
	if position < 0 || position >= MaxDigitsPerDisplay*NumDisplays {
		return // 0-15の範囲外なら何もしない
	}

	display := position / MaxDigitsPerDisplay        // 0-7 -> 0, 8-15 -> 1
	digitInDisplay := position % MaxDigitsPerDisplay // 8 -> 0, 9 -> 1, ...

	d.SetDigitOnDisplay(display, digitInDisplay, num, dot)
}

// ClearOnDisplay clears one of the two 8-digit displays.
// display: 0 for display A, 1 for display B.
//
// ClearOnDisplayは、2つの8桁ディスプレイのうちの1つをクリアする。
func (d *Device) ClearOnDisplay(display int) {
	if display < 0 || display >= NumDisplays {
		return
	}
	for pos := 0; pos < MaxDigitsPerDisplay; pos++ {
		d.SetDigitOnDisplay(display, pos, blankPatternIndex, false)
	}
}

// ClearFadeOnDisplay clears one of the two 8-digit displays with a fade effect.
// It clears the display in the buffer and then performs a fade-out/fade-in.
//
// ClearFadeOnDisplayは、フェード効果付きで2つの8桁ディスプレイのうちの1つをクリアする。
// バッファ内のディスプレイをクリアした後、フェードアウト/フェードインを実行する。
func (d *Device) ClearFadeOnDisplay(display int, delay time.Duration) {
	if display < 0 || display >= NumDisplays {
		return
	}
	d.ClearOnDisplay(display) // Clear the relevant part of the buffer
	d.DisplayFade(delay)      // Apply the fade effect to show the change
}

// ClearAllFade clears both displays with a fade effect.
//
// ClearAllFadeは、フェード効果付きで両方のディスプレイをクリアする。
func (d *Device) ClearAllFade(delay time.Duration) {
	d.ClearAll()
	d.DisplayFade(delay)
}

// WriteString displays a string on one of the two displays.
// It clears the target display before writing.
//
// WriteStringは、2つのディスプレイのいずれかに文字列を表示する。
//
// display: 0 for the first display (A), 1 for the second (B).
// s: The string to display. Handles numbers and dots (e.g., "123", "45.6", "78.").
func (d *Device) WriteString(display int, s string) {
	if display < 0 || display >= NumDisplays {
		return
	}

	d.ClearOnDisplay(display)

	digitPos := 0
	runes := []rune(s) // runeのスライスに変換して、マルチバイト文字にも対応する
	for i := 0; i < len(runes) && digitPos < MaxDigitsPerDisplay; i++ {
		char := runes[i]
		if char >= '0' && char <= '9' {
			num := byte(char - '0')
			dot := false
			// Look ahead for a dot
			if i+1 < len(runes) && runes[i+1] == '.' {
				dot = true
				i++ // ドットを処理したので、次の文字はスキップ
			}

			d.SetDigitOnDisplay(display, digitPos, num, dot)
			digitPos++
		}
		// 数字でもドットでもない文字は、ここでは単純に無視
	}
}

// Display transfers the buffer's content to the LED driver.
//
// Displayは、バッファの内容をLEDドライバに転送する。
func (d *Device) Display() {
	data := append([]byte{0x00}, d.buffer[:]...)
	d.bus.Tx(uint16(d.Address), data, nil)
}

// DisplayFade transfers the buffer's content with a fade-out/fade-in effect.
// It's a visual effect and will block for the duration of the fade.
//
// DisplayFadeは、フェードアウト/フェードイン効果付きでバッファの内容を転送する。
// これは視覚効果であり、フェード中は処理をブロックする。
func (d *Device) DisplayFade(delay time.Duration) {
	// Fade out
	for i := int(d.currentBrightness); i >= 0; i-- {
		d.SetBrightness(uint8(i))
		time.Sleep(delay)
	}

	// Update the display content
	d.Display()

	// Fade in
	for i := 0; i <= 15; i++ {
		d.SetBrightness(uint8(i))
		time.Sleep(delay)
	}
	// Ensure brightness is set to the final desired level
	d.SetBrightness(15)
}

// SetBrightness sets the display brightness (0-15).
//
// SetBrightnessは、ディスプレイの明るさを設定する(0-15)。
func (d *Device) SetBrightness(brightness uint8) {
	if brightness > 15 {
		brightness = 15
	}
	d.currentBrightness = brightness
	d.bus.Tx(uint16(d.Address), []byte{ht16k33SetBrightness | brightness}, nil)
}
