package tui

// uiText is one UI string in English and Chinese.
type uiText [2]string

// lang is the UI language index: 0 English and 1 Chinese.
var lang int

// t returns the string in the active UI language.
func t(text uiText) string {
	return text[lang]
}

// setLang selects the UI language from the state file value.
func setLang(value string) {
	if value == "zh" {
		lang = 1
		return
	}
	lang = 0
}

// Catppuccin Macchiato colors used by the interface.
const (
	colText     = "\x1b[38;2;202;211;245m"
	colTextBold = "\x1b[1m\x1b[38;2;202;211;245m"
	colGreen    = "\x1b[38;2;166;218;149m"
	colHeading  = "\x1b[1m\x1b[38;2;183;189;248m"
)

// UI strings in English and Chinese.
var (
	txtHelpTitle    = uiText{"Keyboard Shortcuts", "快捷键"}
	txtHelpTooSmall = uiText{"Terminal too small to show the keymap", "终端过小，无法显示快捷键"}
	txtHelpLegend   = uiText{"* Only active in input", "* 仅输入时有效"}
	txtHelpBack     = uiText{"Press %s to go back", "按 %s 返回"}
	txtQuitAsk      = uiText{"Are you sure you want to quit?", "确定要退出吗？"}
	txtDeleteAsk    = uiText{"Delete %s?", "确定要删除 %s 吗？"}
	txtBtnQuit      = uiText{" Quit", " 退出"}
	txtBtnDelete    = uiText{" Delete", " 删除"}
	txtBtnCancel    = uiText{" Cancel", " 取消"}

	txtHintStart  = uiText{"start", "开启"}
	txtHintPages  = uiText{"switch page", "换页"}
	txtHintMove   = uiText{"move", "移动"}
	txtHintSwitch = uiText{"switch", "切换"}
	txtHintAdd    = uiText{"add", "新增"}
	txtHintDelete = uiText{"delete", "删除"}
	txtHintSync   = uiText{"sync", "同步"}
	txtHintHelp   = uiText{"help", "帮助"}

	txtLoading         = uiText{"Loading", "加载中"}
	txtTraffic         = uiText{"Traffic ", "流量 "}
	txtSort            = uiText{"Sort ", "排序 "}
	txtTooSmallBrowse  = uiText{"Terminal too small to browse", "终端过小，无法浏览"}
	txtTooSmallDisplay = uiText{"Terminal too small to display", "终端过小，无法显示"}

	txtKernel       = uiText{"Kernel", "内核"}
	txtProfile      = uiText{"Profile", "配置"}
	txtNode         = uiText{"Node", "节点"}
	txtTrafficLabel = uiText{"Traffic", "流量"}
	txtPort         = uiText{"Port", "端口"}
	txtRunning      = uiText{"Running", "运行中"}
	txtStopped      = uiText{"Stopped", "已停止"}
	txtOn           = uiText{"On", "开"}
	txtOff          = uiText{"Off", "关"}
	txtUpload       = uiText{"Upload", "上传"}
	txtDownload     = uiText{"Download", "下载"}
	txtPanelTitle   = uiText{"%s %s/s  total %s ", "%s %s/s  共 %s "}

	txtSortRegion  = uiText{"Region", "按地区"}
	txtSortDefault = uiText{"Default", "默认"}
)
