package parser

import (
	"bytes"
	"log"
	"regexp"
	"strings"
	"unicode"
)

// 从终端输出流中解析用户执行的命令
type CommandParser struct {
	buffer          []rune           // 当前行缓冲区
	commandBuffer   []rune           // 命令缓冲区
	inCommand       bool             // 是否在命令输入状态
	promptDetected  bool             // 是否检测到提示符
	promptPatterns  []*regexp.Regexp // 提示符正则表达式
	onCommandFunc   func(string)     // 命令回调函数
	cursorPos       int              // 光标位置
	savedCursorPos  int              // 保存的光标位置
	lastPromptIndex int              // 最后一个提示符的位置
}

// NewCommandParser 创建命令解析器
func NewCommandParser(onCommand func(string)) *CommandParser {
	return &CommandParser{
		buffer:        make([]rune, 0, 1024),
		commandBuffer: make([]rune, 0, 256),
		onCommandFunc: onCommand,
		cursorPos:     0,
		// 常见的 shell 提示符模式
		promptPatterns: []*regexp.Regexp{
			regexp.MustCompile(`[\$#]\s*$`),                  // 简单提示符: $ 或 #
			regexp.MustCompile(`\[.*?@.*?\s+.*?\][\$#]\s*$`), // [user@host dir]$ 或 [user@host dir]#
			regexp.MustCompile(`.*?@.*?:.*?[\$#]\s*$`),       // user@host:dir$ 或 user@host:dir#
			regexp.MustCompile(`.*?>\s*$`),                   // Windows 风格: >
			regexp.MustCompile(`\w+@[\w\-]+:\S+[\$#]\s*$`),   // user@hostname:/path$
			regexp.MustCompile(`^root@.*?[\$#]\s*$`),         // root@...$ 或 root@...#
			regexp.MustCompile(`^\[root@.*?\]#\s*$`),         // [root@...]#
			regexp.MustCompile(`^bash-[\d.]+[\$#]\s*$`),      // bash-4.2$ 或 bash-4.2#
			regexp.MustCompile(`^sh-[\d.]+[\$#]\s*$`),        // sh-4.2$ 或 sh-4.2#
		},
	}
}

// Feed 输入数据到解析器（从 SSH 输出流）
func (p *CommandParser) Feed(data string) {
	for _, ch := range data {
		p.processChar(ch)
	}
}

// processChar 处理单个字符
func (p *CommandParser) processChar(ch rune) {
	// 处理回车和换行
	if ch == '\r' || ch == '\n' {
		p.handleNewLine()
		return
	}

	// 处理退格键
	if ch == 127 || ch == 8 { // DEL 或 BS
		p.handleBackspace()
		return
	}

	// 处理 ANSI 转义序列
	if ch == 27 { // ESC
		p.handleEscapeSequence()
		return
	}

	// 处理制表符
	if ch == '\t' {
		// Tab 补全可能会改变缓冲区内容
		// 暂时忽略，等待完整输出
		return
	}

	// 处理普通可打印字符
	if unicode.IsPrint(ch) || ch == ' ' {
		// 如果已检测到提示符，说明在命令输入状态
		if p.promptDetected {
			// 在光标位置插入字符
			if p.cursorPos >= len(p.commandBuffer) {
				p.commandBuffer = append(p.commandBuffer, ch)
			} else {
				// 在中间插入字符
				p.commandBuffer = append(p.commandBuffer[:p.cursorPos], append([]rune{ch}, p.commandBuffer[p.cursorPos:]...)...)
			}
			p.cursorPos++
		}

		// 添加到行缓冲区
		if p.cursorPos >= len(p.buffer) {
			p.buffer = append(p.buffer, ch)
		} else {
			p.buffer = append(p.buffer[:p.cursorPos], append([]rune{ch}, p.buffer[p.cursorPos:]...)...)
		}
	}
}

// handleNewLine 处理换行
func (p *CommandParser) handleNewLine() {
	line := string(p.buffer)

	// 如果当前有命令缓冲区，说明用户按下了回车，命令执行
	if p.promptDetected && len(p.commandBuffer) > 0 {
		command := strings.TrimSpace(string(p.commandBuffer))

		// 过滤空命令和特殊命令
		if command != "" && !p.shouldIgnoreCommand(command) {
			log.Printf("[CommandParser] Detected command: %q", command)
			if p.onCommandFunc != nil {
				p.onCommandFunc(command)
			}
		}

		// 重置命令缓冲区
		p.commandBuffer = p.commandBuffer[:0]
		p.promptDetected = false
	}

	// 检查行缓冲区是否包含提示符
	cleanLine := p.stripANSI(line)
	if p.isPrompt(cleanLine) {
		log.Printf("[CommandParser] Detected prompt: %q", cleanLine)
		p.promptDetected = true
		p.lastPromptIndex = len(p.buffer)
		// 清空命令缓冲区，准备接收新命令
		p.commandBuffer = p.commandBuffer[:0]
	}

	// 重置行缓冲区和光标
	p.buffer = p.buffer[:0]
	p.cursorPos = 0
}

// handleBackspace 处理退格
func (p *CommandParser) handleBackspace() {
	if p.promptDetected && p.cursorPos > 0 {
		// 从命令缓冲区删除字符
		if p.cursorPos <= len(p.commandBuffer) {
			p.commandBuffer = append(p.commandBuffer[:p.cursorPos-1], p.commandBuffer[p.cursorPos:]...)
		}
		p.cursorPos--
	}

	// 从行缓冲区删除字符
	if p.cursorPos > 0 && p.cursorPos <= len(p.buffer) {
		p.buffer = append(p.buffer[:p.cursorPos-1], p.buffer[p.cursorPos:]...)
	}
}

// handleEscapeSequence 处理 ANSI 转义序列（光标移动等）
func (p *CommandParser) handleEscapeSequence() {
	// ANSI 转义序列会在 stripANSI 中移除
	// 这里我们主要关注光标移动指令，但由于难以准确跟踪，
	// 我们依赖输出流本身已经处理过的结果
}

// isPrompt 检查是否为命令提示符
func (p *CommandParser) isPrompt(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}

	// 使用正则表达式匹配常见提示符
	for _, pattern := range p.promptPatterns {
		if pattern.MatchString(line) {
			return true
		}
	}

	return false
}

// stripANSI 移除 ANSI 转义序列
func (p *CommandParser) stripANSI(s string) string {
	// ANSI 转义序列正则: ESC[ 开头，以字母结束
	ansiPattern := regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	s = ansiPattern.ReplaceAllString(s, "")

	// 移除其他控制字符（保留空格和可打印字符）
	s = strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) || r == ' ' || r == '\t' {
			return r
		}
		return -1
	}, s)

	return s
}

// shouldIgnoreCommand 判断是否应该忽略该命令
func (p *CommandParser) shouldIgnoreCommand(cmd string) bool {
	// 忽略的命令列表
	ignoreList := []string{
		"exit",
		"logout",
		"clear",
		"reset",
		"history",
		"pwd",
		"", // 空命令
	}

	cmd = strings.TrimSpace(cmd)
	for _, ignore := range ignoreList {
		if cmd == ignore {
			return true
		}
	}

	// 忽略只包含空格的命令
	if strings.TrimSpace(cmd) == "" {
		return true
	}

	return false
}

// Reset 重置解析器状态
func (p *CommandParser) Reset() {
	p.buffer = p.buffer[:0]
	p.commandBuffer = p.commandBuffer[:0]
	p.inCommand = false
	p.promptDetected = false
	p.cursorPos = 0
	p.lastPromptIndex = 0
}

// CommandExtractor 命令提取器 - 改进版
// 策略：收集输出行，当检测到新提示符时，提取上一条命令
type CommandExtractor struct {
	lines             []string     // 收集的输出行
	lineBuffer        bytes.Buffer // 当前行缓冲
	onCommandFunc     func(string)
	lastPromptIdx     int  // 上一个提示符的索引
	waitingForCommand bool // 是否在等待命令输入（检测到提示符后的状态）
}

// NewCommandExtractor 创建命令提取器
func NewCommandExtractor(onCommand func(string)) *CommandExtractor {
	return &CommandExtractor{
		lines:             make([]string, 0, 100),
		onCommandFunc:     onCommand,
		lastPromptIdx:     -1,
		waitingForCommand: false,
	}
}

// Feed 输入数据
func (e *CommandExtractor) Feed(data string) {
	i := 0
	runes := []rune(data)

	for i < len(runes) {
		ch := runes[i]

		if ch == '\r' {
			// 遇到 \r，先不处理，可能后面跟着 \n
			i++
			continue
		} else if ch == '\n' {
			e.processLine()
			i++
		} else if ch == 8 || ch == 127 { // 退格符 (BS=8, DEL=127)
			// 从缓冲区删除最后一个字符
			s := e.lineBuffer.String()
			if len(s) > 0 {
				rs := []rune(s)
				if len(rs) > 0 {
					e.lineBuffer.Reset()
					e.lineBuffer.WriteString(string(rs[:len(rs)-1]))
				}
			}
			// 跳过可能跟随的 ANSI 序列 (如 \x1b[K)
			i++
			if i < len(runes) && runes[i] == 27 { // ESC
				// 跳过整个 ANSI 转义序列
				i++
				if i < len(runes) && runes[i] == '[' {
					i++
					// 跳过直到遇到字母
					for i < len(runes) && !((runes[i] >= 'A' && runes[i] <= 'Z') || (runes[i] >= 'a' && runes[i] <= 'z')) {
						i++
					}
					if i < len(runes) {
						i++ // 跳过结束字母
					}
				}
			}
		} else if ch == 27 { // ESC，ANSI 转义序列的开始
			// 跳过整个 ANSI 转义序列，不加入缓冲区
			i++
			if i < len(runes) {
				if runes[i] == '[' {
					i++
					// 跳过参数直到遇到字母
					for i < len(runes) && !((runes[i] >= 'A' && runes[i] <= 'Z') || (runes[i] >= 'a' && runes[i] <= 'z')) {
						i++
					}
					if i < len(runes) {
						i++ // 跳过结束字母
					}
				} else if runes[i] == ']' {
					// OSC 序列 (如 \x1b]0;...\a)
					i++
					for i < len(runes) && runes[i] != 7 && runes[i] != '\r' && runes[i] != '\n' {
						i++
					}
					if i < len(runes) && runes[i] == 7 {
						i++
					}
				} else {
					i++
				}
			}
		} else if ch == 3 { // Ctrl+C
			// 清空当前缓冲区，不记录被取消的命令
			e.lineBuffer.Reset()
			i++
		} else {
			e.lineBuffer.WriteRune(ch)
			i++
		}
	}
}

// processLine 处理完整的一行
func (e *CommandExtractor) processLine() {
	line := e.lineBuffer.String()
	e.lineBuffer.Reset()

	if line == "" {
		return
	}

	// 移除 ANSI 转义序列
	cleanLine := stripANSISimple(line)
	cleanLine = strings.TrimSpace(cleanLine)

	if cleanLine == "" {
		return
	}

	// 检查是否包含提示符
	hasPrompt := containsPromptMarker(cleanLine)

	if hasPrompt {
		// 直接从包含提示符的行中提取命令（提示符和命令在同一行）
		cmd := extractCommandFromLine(cleanLine)
		if cmd != "" && !shouldIgnoreSimple(cmd) {
			if e.onCommandFunc != nil {
				e.onCommandFunc(cmd)
			}
			e.waitingForCommand = false
		} else if cmd != "" && shouldIgnoreSimple(cmd) {
			// 命令被忽略了（如 clear），不等待下一行
			e.waitingForCommand = false
		} else {
			// 只有提示符，没有命令，标记为等待命令状态
			e.waitingForCommand = true
		}
	} else if e.waitingForCommand {
		// 之前检测到提示符，现在这行可能是命令或输出
		// 判断是否为有效命令
		isOutput := looksLikeOutput(cleanLine)
		isIgnored := shouldIgnoreSimple(cleanLine)
		isCommand := looksLikeCommand(cleanLine)

		if isOutput {
			e.waitingForCommand = false
		} else if isIgnored {
			e.waitingForCommand = false
		} else if isCommand {
			if e.onCommandFunc != nil {
				e.onCommandFunc(cleanLine)
			}
			e.waitingForCommand = false
		} else {
			// 不像命令，可能是输出
			e.waitingForCommand = false
		}
	}

	// 保存当前行（用于调试）
	e.lines = append(e.lines, cleanLine)

	// 保持最近 20 行，避免内存增长
	if len(e.lines) > 20 {
		e.lines = e.lines[len(e.lines)-20:]
	}
}

// containsPromptMarker 检查行是否包含提示符标记（通用方法，不依赖特定格式）
// 策略：识别 prompt 的共同特征，而不是匹配特定格式
func containsPromptMarker(line string) bool {
	// 必须包含提示符结束标记（$ 或 #）
	// 支持多种格式：$ 、# 、]$ 、]# 或行尾的 $、#、]$、]#
	hasMarker := strings.Contains(line, "$ ") ||
		strings.Contains(line, "# ") ||
		strings.Contains(line, "]$ ") ||
		strings.Contains(line, "]# ") ||
		strings.HasSuffix(line, "$") ||
		strings.HasSuffix(line, "#") ||
		strings.HasSuffix(line, "]$") ||
		strings.HasSuffix(line, "]#")

	if !hasMarker {
		return false
	}

	// 通用检测：找到所有可能的 prompt 标记位置
	// 检查标记前的内容是否看起来像 prompt（而不是命令的一部分）
	promptMarkers := []string{"]# ", "]$ ", "# ", "$ "}
	
	for _, marker := range promptMarkers {
		idx := strings.LastIndex(line, marker)
		if idx >= 0 {
			beforeMarker := line[:idx]
			// 检查标记前的内容是否像 prompt
			if looksLikePromptPrefix(beforeMarker) {
				return true
			}
		}
	}

	// 检查行尾的标记
	endMarkers := []string{"]#", "]$", "#", "$"}
	for _, marker := range endMarkers {
		if strings.HasSuffix(line, marker) {
			beforeMarker := line[:len(line)-len(marker)]
			if looksLikePromptPrefix(beforeMarker) {
				return true
			}
		}
	}

	return false
}

// looksLikePromptPrefix 检查字符串是否看起来像 prompt 的前缀部分
// 不依赖特定格式，而是识别 prompt 的共同特征
func looksLikePromptPrefix(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}

	// Prompt 的共同特征：
	// 1. 通常包含标识符（hostname、username、container ID 等）
	// 2. 可能包含路径分隔符（: 或 /）
	// 3. 可能包含 @ 符号（传统格式：user@host）
	// 4. 不应该包含太多空格（prompt 通常是紧凑的）
	// 5. 不应该看起来像命令输出

	// 检查是否包含太多空格（prompt 通常是紧凑的）
	spaceCount := strings.Count(s, " ")
	if spaceCount > 2 {
		// 太多空格，可能是命令输出而不是 prompt
		return false
	}

	// 优先检查：包含 @ 符号通常是传统 prompt 格式（user@host:path）
	hasAtSymbol := strings.Contains(s, "@")
	
	// 检查是否包含路径分隔符（: 或 /）
	hasPathSeparator := strings.Contains(s, ":") || strings.Contains(s, "/")
	
	// 检查是否包含常见的标识符字符（字母、数字、连字符、下划线）
	hasIdentifier := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || 
		   (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			hasIdentifier = true
			break
		}
	}

	// 如果包含 @ 符号，且空格不多，很可能是 prompt
	if hasAtSymbol && spaceCount <= 2 {
		// 进一步检查：不应该看起来像命令输出
		if !looksLikeOutput(s) {
			return true
		}
	}

	// 如果包含路径分隔符或标识符，且空格不多，可能是 prompt
	if (hasPathSeparator || hasIdentifier) && spaceCount <= 2 {
		// 进一步检查：不应该看起来像命令输出
		if !looksLikeOutput(s) {
			return true
		}
	}

	// 特殊情况：只有标识符，没有路径分隔符（如简单的 hostname$）
	if !hasPathSeparator && hasIdentifier && spaceCount == 0 && len(s) > 0 {
		// 检查是否主要是标识符字符
		identifierChars := 0
		for _, r := range s {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || 
			   (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
				identifierChars++
			}
		}
		// 如果大部分是标识符字符，可能是 prompt
		if identifierChars > len(s)/2 {
			return true
		}
	}

	return false
}

// extractCommandFromLine 从包含提示符的行中提取命令（通用方法）
// 策略：找到最后一个有效的 prompt 标记，从标记后提取命令
func extractCommandFromLine(line string) string {
	// 如果行中包含多个提示符（如清屏后的情况），使用最后一个
	// 例如: "root@host:# root@host:# pwd" -> 应该提取 "pwd"
	markersWithSpace := []string{"]# ", "]$ ", "# ", "$ "}

	// 找到所有可能的提示符标记位置，使用最后一个有效的
	var lastMarkerPos int = -1
	var lastMarker string

	for _, m := range markersWithSpace {
		idx := 0
		for {
			pos := strings.Index(line[idx:], m)
			if pos < 0 {
				break
			}
			pos += idx
			beforeMarker := line[:pos]
			
			// 使用通用的 prompt 检测方法
			if looksLikePromptPrefix(beforeMarker) {
				lastMarkerPos = pos
				lastMarker = m
			}
			
			idx = pos + len(m)
		}
	}

	// 如果找到了提示符标记
	if lastMarkerPos >= 0 {
		cmdStart := lastMarkerPos + len(lastMarker)
		if cmdStart >= len(line) {
			// 提示符后面没有内容，只是提示符
			return ""
		}

		cmd := strings.TrimSpace(line[cmdStart:])

		// 检查提取的命令是否又是一个提示符（不应该）
		if isPromptPattern(cmd) {
			return ""
		}

		return cmd
	}

	// 如果没找到带空格的标记，尝试末尾标记（提示符单独一行）
	endMarkers := []string{"]#", "]$", "#", "$"}
	for _, m := range endMarkers {
		if strings.HasSuffix(line, m) {
			beforeMarker := line[:len(line)-len(m)]
			// 使用通用的 prompt 检测方法
			if looksLikePromptPrefix(beforeMarker) {
				// 提示符单独一行，没有命令
				return ""
			}
		}
	}

	return ""
}

// looksLikeOutput 判断行是否看起来像命令输出
func looksLikeOutput(line string) bool {
	// 输出的特征：
	// - 以空格开头（缩进）
	// - 包含 "total"（ls 输出）
	// - 包含权限字符串（drwxr-xr-x）
	// - 包含常见错误提示
	// - 很长的行（超过 200 字符）
	// - 以 / 开头的路径（通常是 pwd 或 cd 的输出）
	// - 只包含 @ 和 : 的行（可能是提示符片段或 whoami 输出）

	if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
		return true
	}

	if strings.HasPrefix(line, "total ") {
		return true
	}

	// 权限字符串模式（ls -l 输出）
	if len(line) > 0 && (line[0] == 'd' || line[0] == '-' || line[0] == 'l') {
		if len(line) > 10 && strings.Count(line[:10], "r")+strings.Count(line[:10], "w")+strings.Count(line[:10], "x") >= 3 {
			return true
		}
	}

	// 以 / 开头的绝对路径（pwd 输出）
	if strings.HasPrefix(line, "/") && !strings.Contains(line, " ") {
		return true
	}

	// 包含 @ 和以 : 结尾的行（提示符片段或 whoami 输出）
	if strings.Contains(line, "@") && strings.HasSuffix(line, ":") {
		return true
	}

	// 常见的命令输出前缀
	outputPrefixes := []string{
		"bash:", "sh:", "-bash:", "-sh:",
		"command not found",
		"No such file or directory",
		"Permission denied",
	}

	for _, prefix := range outputPrefixes {
		if strings.Contains(line, prefix) {
			return true
		}
	}

	// 超长行可能是输出
	if len(line) > 200 {
		return true
	}

	return false
}

// looksLikeCommand 判断行是否看起来像命令
func looksLikeCommand(line string) bool {
	line = strings.TrimSpace(line)

	// 空行不是命令
	if line == "" {
		return false
	}

	// 以 / 开头且不包含空格的可能是路径输出，不是命令
	if strings.HasPrefix(line, "/") && !strings.Contains(line, " ") {
		return false
	}

	// 包含 @ 和以 : 结尾的是提示符片段，不是命令
	// 支持多种格式：root@host:, root@host:~, root@host:/path
	if strings.Contains(line, "@") && strings.Contains(line, ":") {
		// 检查是否是提示符格式（包含 @ 且以 : 结尾，没有后续命令）
		colonIdx := strings.LastIndex(line, ":")
		if colonIdx >= 0 {
			afterColon := line[colonIdx+1:]
			// 如果冒号后面没有空格，可能是提示符（如 root@host: 或 root@host:/path）
			if !strings.Contains(afterColon, " ") {
				// 进一步验证：如果整行不包含空格或匹配提示符模式，认为是提示符
				if !strings.Contains(line, " ") || isPromptPattern(line) {
					return false
				}
			}
		}
	}

	// 常见的命令特征：
	// 1. 以常见命令开头
	commonCommands := []string{
		"ls", "cd", "pwd", "cat", "echo", "grep", "find", "ps", "top", "df", "du",
		"mkdir", "rm", "cp", "mv", "touch", "chmod", "chown", "tar", "gzip", "zip",
		"ssh", "scp", "curl", "wget", "ping", "netstat", "ifconfig", "ip", "route",
		"systemctl", "service", "docker", "kubectl", "git", "npm", "yarn", "python",
		"java", "go", "make", "gcc", "vim", "nano", "less", "more", "head", "tail",
		"awk", "sed", "sort", "uniq", "wc", "diff", "vi", "su", "sudo",
	}

	firstWord := line
	if idx := strings.Index(line, " "); idx > 0 {
		firstWord = line[:idx]
	}

	// 检查是否是常见命令
	for _, cmd := range commonCommands {
		if firstWord == cmd {
			return true
		}
	}

	// 2. 包含常见命令字符但不像输出
	// 如果行长度合理（不太长）且包含字母数字，可能是命令
	if len(line) < 100 && !looksLikeOutput(line) {
		// 检查是否主要由字母、数字、常见符号组成
		validChars := 0
		for _, ch := range line {
			if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
				(ch >= '0' && ch <= '9') || ch == '-' || ch == '_' ||
				ch == '.' || ch == '/' || ch == ' ' {
				validChars++
			}
		}

		// 如果大部分字符都是有效的命令字符
		if float64(validChars)/float64(len(line)) > 0.8 {
			return true
		}
	}

	return false
}

// stripANSISimple 简单移除 ANSI 转义序列
func stripANSISimple(s string) string {
	// 移除所有 ANSI/VT100 转义序列
	// ESC [ ... 字母
	ansiPattern := regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)
	s = ansiPattern.ReplaceAllString(s, "")

	// ESC ] ... (OSC 序列)
	oscPattern := regexp.MustCompile(`\x1b\][^\x07]*\x07`)
	s = oscPattern.ReplaceAllString(s, "")

	// 移除其他 ESC 序列
	escPattern := regexp.MustCompile(`\x1b[^[]`)
	s = escPattern.ReplaceAllString(s, "")

	// 移除其他控制字符（保留空格、制表符）
	result := strings.Builder{}
	for _, r := range s {
		if unicode.IsPrint(r) || r == ' ' || r == '\t' {
			result.WriteRune(r)
		}
	}

	return result.String()
}

// isPromptPattern 检查是否是提示符模式（通用方法）
// 不依赖特定格式，而是识别 prompt 的共同特征
func isPromptPattern(line string) bool {
	// 必须包含提示符标记
	hasMarker := strings.HasSuffix(line, "$") || 
		strings.HasSuffix(line, "#") ||
		strings.HasSuffix(line, "]$") ||
		strings.HasSuffix(line, "]#") ||
		strings.Contains(line, "$ ") ||
		strings.Contains(line, "# ")

	if !hasMarker {
		return false
	}

	// 移除提示符标记，检查剩余部分是否像 prompt 前缀
	// 找到最后一个提示符标记的位置
	markers := []string{"]# ", "]$ ", "# ", "$ ", "]#", "]$", "#", "$"}
	for _, marker := range markers {
		if strings.HasSuffix(line, marker) {
			beforeMarker := line[:len(line)-len(marker)]
			if looksLikePromptPrefix(beforeMarker) {
				return true
			}
		} else if strings.Contains(line, marker) {
			idx := strings.LastIndex(line, marker)
			if idx >= 0 {
				beforeMarker := line[:idx]
				if looksLikePromptPrefix(beforeMarker) {
					return true
				}
			}
		}
	}

	return false
}

// shouldIgnoreSimple 简单的忽略判断
func shouldIgnoreSimple(cmd string) bool {
	ignoreList := []string{"exit", "logout", "clear", "reset"}
	cmd = strings.TrimSpace(strings.ToLower(cmd))

	// 忽略空命令
	if cmd == "" {
		return true
	}

	// 忽略包含 Ctrl+C 的命令
	if strings.Contains(cmd, "^C") || strings.Contains(cmd, "^c") {
		return true
	}

	// 忽略提示符本身（防止提示符被当作命令记录）
	if isPromptPattern(cmd) {
		return true
	}

	// 忽略只包含 user@host: 格式的行（提示符片段）
	if strings.Contains(cmd, "@") && strings.Contains(cmd, ":") && !strings.Contains(cmd, " ") {
		return true
	}

	// 忽略列表中的命令
	for _, ignore := range ignoreList {
		if cmd == ignore {
			return true
		}
	}

	return false
}
