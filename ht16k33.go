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

// fadeState represents the current state of the non-blocking fade effect.
type fadeState uint8

const (
	fadeStateIdle fadeState = iota
	fadeStateOut
	fadeStateIn
)

// 7-segment display number patterns (g-f-e-d-c-b-a) plus a blank pattern.
// A map is more flexible for alphanumeric characters.
//
// マップは、英数字を扱いやすい。
var font = map[rune]byte{
	'0':  0b00111111,
	'1':  0b00000110,
	'2':  0b01011011,
	'3':  0b01001111,
	'4':  0b01100110,
	'5':  0b01101101,
	'6':  0b01111101,
	'7':  0b00000111,
	'8':  0b01111111,
	'9':  0b01101111,
	'A':  0b01110111,
	'B':  0b01111100, // Lowercase 'b'
	'C':  0b00111001,
	'D':  0b01011110, // Lowercase 'd'
	'E':  0b01111001,
	'F':  0b01110001,
	'G':  0b00111101,
	'H':  0b01110110,
	'I':  0b00000110, // Same as 1
	'J':  0b00011110,
	'L':  0b00111000,
	'O':  0b00111111, // Same as 0
	'P':  0b01110011,
	'Q':  0b01100111,
	'R':  0b01010000, // Lowercase 'r'
	'S':  0b01101101, // Same as 5
	'U':  0b00111110,
	'Y':  0b01101110,
	' ':  0b00000000, // Space
	'-':  0b01000000, // Minus
	'_':  0b00001000, // Underscore
	'\'': 0b00000010, // Apostrophe
	'"':  0b00100010, // Double quote
	'=':  0b01001000, // Equals
	'?':  0b01010011, // Question mark
	// Add more characters as needed!
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

	// --- For non-blocking fade ---
	fadeState      fadeState
	fadeStep       int
	lastUpdateTime time.Time
	fadeDelay      time.Duration
}

// New creates a new Device instance.
//
// Newは、新しいDeviceインスタンスを作る
func New(bus I2CBus, address uint8) Device {
	return Device{
		bus:               bus,
		Address:           address,
		currentBrightness: 15, // Default to max brightness
		fadeState:         fadeStateIdle,
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

	// For SetDigitOnDisplay, we still need to handle the old `num` byte argument.
	// We'll map it to a character.
	char := rune('0' + num)
	pattern, _ := font[char] // If num > 9, it won't be in the map, pattern will be 0 (blank)

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
func (d *Device) ClearFadeOnDisplayBlocking(display int, delay time.Duration) {
	if display < 0 || display >= NumDisplays {
		return
	}
	d.ClearOnDisplay(display)    // Clear the relevant part of the buffer
	d.DisplayFadeBlocking(delay) // Apply the fade effect to show the change
}

// ClearAllFade clears both displays with a fade effect.
//
// ClearAllFadeは、フェード効果付きで両方のディスプレイをクリアする。
func (d *Device) ClearAllFadeBlocking(delay time.Duration) {
	d.ClearAll()
	d.DisplayFadeBlocking(delay)
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
		// Convert to uppercase to match the font map keys
		char := runes[i]
		if char >= 'a' && char <= 'z' {
			char = char - 'a' + 'A'
		}

		if pattern, ok := font[char]; ok {
			dot := false
			// Look ahead for a dot
			if i+1 < len(runes) && runes[i+1] == '.' {
				dot = true
				i++ // ドットを処理したので、次の文字はスキップ
			}
			d.setPattern(display, digitPos, pattern, dot)
			digitPos++
		} // If character is not in the font map, it's ignored.
	}
}

// setPattern is a helper to directly set a segment pattern at a position.
//
// setPatternは、指定した位置にセグメントパターンを直接設定するためのヘルパー関数。
func (d *Device) setPattern(display int, position int, pattern byte, dot bool) {
	if display < 0 || display >= NumDisplays || position < 0 || position >= MaxDigitsPerDisplay {
		return
	}

	rowOffset := display * MaxDigitsPerDisplay
	mask := ^byte(1 << position)

	// Clear the bits for this digit position first
	for i := 0; i < MaxDigitsPerDisplay; i++ {
		d.buffer[rowOffset+i] &= mask
	}

	// Set the new segment bits
	for seg := 0; seg < 7; seg++ {
		if (pattern>>seg)&1 == 1 {
			d.buffer[rowOffset+seg] |= (1 << position)
		}
	}

	// Set the new dot bit
	if dot {
		dotRow := rowOffset + 7
		d.buffer[dotRow] |= (1 << position)
	}
}

// Display transfers the buffer's content to the LED driver.
//
// Displayは、バッファの内容をLEDドライバに転送する。
func (d *Device) Display() {
	data := append([]byte{0x00}, d.buffer[:]...)
	d.bus.Tx(uint16(d.Address), data, nil)
}

// LightUpAll turns on all segments of all digits on both displays.
// This effectively makes the displays act as a simple light source.
//
// LightUpAllは、両方のディスプレイのすべての桁のすべてのセグメントを点灯させる。
// これにより、ディスプレイが単純な光源として機能するようになる。
func (d *Device) LightUpAll() {
	for i := range d.buffer {
		d.buffer[i] = 0xFF // Turn on all 8 digits for this segment row
	}
}

// LightUpAllFadeBlocking turns on all segments with a fade-in effect.
// This is a blocking function.
//
// LightUpAllFadeBlockingは、フェードイン効果付きですべてのセグメントを点灯させる。
// これはブロッキング関数。
func (d *Device) LightUpAllFadeBlocking(delay time.Duration) {
	d.LightUpAll()
	for i := 0; i <= 15; i++ {
		d.SetBrightness(uint8(i))
		time.Sleep(delay)
	}
}

// DisplayFadeBlocking is a blocking version of the fade effect.
// For non-blocking behavior, use StartFade() and UpdateFade() instead.
//
// DisplayFadeBlockingは、ブロッキング版のフェード効果。
// ノンブロッキングで動かすには、代わりにStartFade()とUpdateFade()を使う。
func (d *Device) DisplayFadeBlocking(delay time.Duration) {
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

// StartFade initiates a non-blocking fade effect.
// Call UpdateFade() repeatedly in your main loop to drive the animation.
//
// StartFadeは、ノンブロッキングのフェード効果を開始する。
// アニメーションを動かすには、メインループでUpdate()を繰り返し呼び出す。
func (d *Device) StartFade(delay time.Duration) {
	if d.fadeState != fadeStateIdle {
		return // Already fading
	}
	d.fadeDelay = delay
	d.fadeState = fadeStateOut
	d.fadeStep = int(d.currentBrightness)
	d.lastUpdateTime = time.Now()
}

// UpdateFade drives the non-blocking fade animation.
// It should be called frequently from the main application loop.
// Returns true if the device is currently in a fade animation.
//
// UpdateFadeは、ノンブロッキングのフェードアニメーションを動かす。
// アプリケーションのメインループから頻繁に呼び出す必要がある。
// フェードアニメーション中はtrueを返す。
func (d *Device) UpdateFade() bool {
	if d.fadeState == fadeStateIdle || time.Since(d.lastUpdateTime) < d.fadeDelay {
		return d.IsFading()
	}

	d.lastUpdateTime = time.Now()

	switch d.fadeState {
	case fadeStateOut:
		d.SetBrightness(uint8(d.fadeStep))
		d.fadeStep--
		if d.fadeStep < 0 {
			d.Display() // Switch content when fully faded out
			d.fadeState = fadeStateIn
			d.fadeStep = 0
		}
	case fadeStateIn:
		d.SetBrightness(uint8(d.fadeStep))
		d.fadeStep++
		if d.fadeStep > 15 {
			d.fadeState = fadeStateIdle // Fade finished
		}
	}
	return d.IsFading()
}

// IsFading returns true if the device is currently in a non-blocking fade animation.
//
// IsFadingは、デバイスがノンブロッキングのフェードアニメーション中であればtrueを返す。
func (d *Device) IsFading() bool {
	return d.fadeState != fadeStateIdle
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
