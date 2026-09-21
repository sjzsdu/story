package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sjzsdu/story/internal/app"
	"github.com/sjzsdu/story/internal/domain"
)

var (
	voiceName        string
	voiceBlID        string
	voiceProvider    string
	voiceModel       string
	voiceInstruction string
	voiceRate        float64
	voicePitch       float64
	voiceStyleNote   string
	voicePreviewText string

	// 造声（声音设计/声音复刻）专属参数。
	voiceBuildPrompt     string
	voiceCloneAudio      string
	voiceCloneAudioURL   string
	voiceBuildLanguage   string
	voiceCloneMaxAudio   float64
	voiceClonePreprocess bool
)

var voiceCmd = &cobra.Command{
	Use:   "voice",
	Short: "管理声音库（与 Series 同级的顶层实体，§16）",
}

var voiceListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出全部声音条目（内置在前）",
	RunE: func(cmd *cobra.Command, args []string) error {
		vs, err := application.ListVoices(rootCtx)
		if err != nil {
			return err
		}
		if len(vs) == 0 {
			fmt.Println("（暂无声音条目，使用 `story voice add --name <名称> --voice <bl-voice-id>` 创建）")
			return nil
		}
		fmt.Printf("%-20s %-14s %-14s %-10s %s\n", "ID", "名称", "BL音色", "类型", "风格说明")
		for _, v := range vs {
			typ := "用户"
			if v.IsBuiltin {
				typ = "内置"
			}
			fmt.Printf("%-20s %-14s %-14s %-10s %s\n", v.ID, truncate(v.Name, 12), v.Voice, typ, truncate(v.StyleNote, 30))
		}
		return nil
	},
}

var voiceShowCmd = &cobra.Command{
	Use:   "show <voice-id>",
	Short: "查看声音条目详情",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		v, err := application.GetVoice(rootCtx, args[0])
		if err != nil {
			return err
		}
		out, _ := json.MarshalIndent(v, "", "  ")
		fmt.Println(string(out))
		return nil
	},
}

var voiceAddCmd = &cobra.Command{
	Use:   "add",
	Short: "新建一个声音条目",
	RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(voiceName) == "" {
			return fmt.Errorf("--name 不能为空")
		}
		if strings.TrimSpace(voiceBlID) == "" {
			return fmt.Errorf("--voice 不能为空（百炼语音 ID，如 longtian_v3）")
		}
		v, err := application.CreateVoice(rootCtx, app.CreateVoiceInput{
			Name:        voiceName,
			Provider:    voiceProvider,
			Voice:       voiceBlID,
			Model:       voiceModel,
			Instruction: voiceInstruction,
			Rate:        voiceRate,
			Pitch:       voicePitch,
			StyleNote:   voiceStyleNote,
		})
		if err != nil {
			return err
		}
		fmt.Printf("声音已创建: %s\n", v.ID)
		fmt.Printf("  名称: %s\n", v.Name)
		fmt.Printf("  BL音色: %s\n", v.Voice)
		if v.Model != "" {
			fmt.Printf("  驱动模型: %s\n", v.Model)
		}
		if v.Rate > 0 {
			fmt.Printf("  语速: %.2f\n", v.Rate)
		}
		if v.Pitch > 0 {
			fmt.Printf("  音高: %.2f\n", v.Pitch)
		}
		if v.Instruction != "" {
			fmt.Printf("  风格指令: %s\n", v.Instruction)
		}
		return nil
	},
}

var voiceEditCmd = &cobra.Command{
	Use:   "edit <voice-id>",
	Short: "编辑声音条目（改后影响所有引用它的系列）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		existing, err := application.GetVoice(rootCtx, id)
		if err != nil {
			return err
		}
		// 部分更新：flag 未显式设置时保留原值（cobra 对 string 默认 "" 难区分未设，
		// 这里按"非空才覆盖"约定；如需清空字段用 server PUT 接口）。
		if cmd.Flags().Changed("name") {
			existing.Name = voiceName
		}
		if cmd.Flags().Changed("provider") {
			existing.Provider = voiceProvider
		}
		if cmd.Flags().Changed("voice") {
			existing.Voice = voiceBlID
		}
		if cmd.Flags().Changed("model") {
			existing.Model = voiceModel
		}
		if cmd.Flags().Changed("instruction") {
			existing.Instruction = voiceInstruction
		}
		if cmd.Flags().Changed("rate") {
			existing.Rate = voiceRate
		}
		if cmd.Flags().Changed("pitch") {
			existing.Pitch = voicePitch
		}
		if cmd.Flags().Changed("style-note") {
			existing.StyleNote = voiceStyleNote
		}
		if err := application.UpdateVoice(rootCtx, existing); err != nil {
			return err
		}
		fmt.Printf("声音已更新: %s\n", existing.ID)
		return nil
	},
}

var voiceRmCmd = &cobra.Command{
	Use:   "rm <voice-id>",
	Short: "删除声音条目（内置不可删；被系列引用时拒绝）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := application.DeleteVoice(rootCtx, args[0]); err != nil {
			return err
		}
		fmt.Printf("声音已删除: %s\n", args[0])
		return nil
	},
}

var voicePreviewCmd = &cobra.Command{
	Use:   "preview <voice-id>",
	Short: "用指定声音合成一段样音（按次计费）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		v, err := application.GetVoice(rootCtx, id)
		if err != nil {
			return err
		}
		text := voicePreviewText
		if strings.TrimSpace(text) == "" {
			text = "话说天下大势，分久必合，合久必分。"
		}
		out, err := application.PreviewVoice(rootCtx, v.ToProfile(), text)
		if err != nil {
			return err
		}
		fmt.Printf("样音已生成: %s\n", out)
		return nil
	},
}

// parseLanguageHints 解析逗号分隔的语种提示，留空返回 nil（provider 用默认 zh）。
func parseLanguageHints(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

var voiceDesignCmd = &cobra.Command{
	Use:   "design",
	Short: "声音设计：用文字描述生成全新音色并落成声音条目（按新建音色个数计费）",
	Long: "调用百炼声音设计（Voice Design）：用文字描述目标音色，生成全新音色并自动落成一条声音条目。\n" +
		"注意：造出的音色必须用造声时的驱动模型（--model，默认 cosyvoice-v3-flash）合成，否则合成会失败；\n" +
		"该模型已写入声音条目，后续引擎合成本会自动使用。",
	RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(voiceName) == "" {
			return fmt.Errorf("--name 不能为空")
		}
		if strings.TrimSpace(voiceBuildPrompt) == "" {
			return fmt.Errorf("--prompt 不能为空（描述目标音色）")
		}
		text := voicePreviewText
		if strings.TrimSpace(text) == "" {
			text = "话说天下大势，分久必合，合久必分。"
		}
		res, err := application.BuildVoice(rootCtx, app.BuildVoiceInput{
			Kind:          domain.VoiceBuildDesign,
			Provider:      voiceProvider,
			Name:          voiceName,
			Prompt:        voiceBuildPrompt,
			PreviewText:   text,
			TargetModel:   voiceModel,
			LanguageHints: parseLanguageHints(voiceBuildLanguage),
			StyleNote:     voiceStyleNote,
		})
		if err != nil {
			return err
		}
		printBuiltVoice(res)
		return nil
	},
}

var voiceCloneCmd = &cobra.Command{
	Use:   "clone",
	Short: "声音复刻：用上传的音频克隆音色并落成声音条目（按新建音色个数计费）",
	Long: "调用百炼声音复刻（Voice Clone）：上传一段参考音频克隆音色，并自动落成一条声音条目。\n" +
		"--audio 传本地音频文件（内部走 bl file upload 上传），或 --audio-url 直接给公网/oss URL。\n" +
		"合规提醒：禁止克隆他人真人声音（声音权风险）。",
	RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(voiceName) == "" {
			return fmt.Errorf("--name 不能为空")
		}
		if strings.TrimSpace(voiceCloneAudio) == "" && strings.TrimSpace(voiceCloneAudioURL) == "" {
			return fmt.Errorf("--audio 或 --audio-url 至少填一个")
		}
		res, err := application.BuildVoice(rootCtx, app.BuildVoiceInput{
			Kind:                 domain.VoiceBuildClone,
			Provider:             voiceProvider,
			Name:                 voiceName,
			AudioPath:            voiceCloneAudio,
			AudioURL:             voiceCloneAudioURL,
			TargetModel:          voiceModel,
			LanguageHints:        parseLanguageHints(voiceBuildLanguage),
			MaxPromptAudioLength: voiceCloneMaxAudio,
			EnablePreprocess:     voiceClonePreprocess,
			StyleNote:            voiceStyleNote,
		})
		if err != nil {
			return err
		}
		printBuiltVoice(res)
		return nil
	},
}

// printBuiltVoice 打印造声结果（声音条目 + 试听音频路径）。
func printBuiltVoice(res *app.BuildVoiceResult) {
	fmt.Printf("声音已创建: %s\n", res.Voice.ID)
	fmt.Printf("  名称: %s\n", res.Voice.Name)
	fmt.Printf("  音色 ID: %s\n", res.Voice.Voice)
	fmt.Printf("  驱动模型: %s\n", res.Voice.Model)
	if res.PreviewAudioPath != "" {
		fmt.Printf("  试听: %s\n", res.PreviewAudioPath)
	}
}

func init() {
	voiceAddCmd.Flags().StringVar(&voiceName, "name", "", "声音显示名（必填）")
	voiceAddCmd.Flags().StringVar(&voiceProvider, "provider", "", "TTS 供应商，默认 bailian")
	voiceAddCmd.Flags().StringVar(&voiceBlID, "voice", "", "百炼语音 ID，如 longtian_v3（必填）")
	voiceAddCmd.Flags().StringVar(&voiceModel, "model", "", "驱动模型（造声音色必填，普通音色留空）")
	voiceAddCmd.Flags().StringVar(&voiceInstruction, "instruction", "", "风格指令（部分音色不支持，会自动降级）")
	voiceAddCmd.Flags().Float64Var(&voiceRate, "rate", 0, "语速 0.5-2.0，0=音色默认")
	voiceAddCmd.Flags().Float64Var(&voicePitch, "pitch", 0, "音高 0.5-2.0，0=音色默认")
	voiceAddCmd.Flags().StringVar(&voiceStyleNote, "style-note", "", "风格说明（仅展示用）")

	voiceEditCmd.Flags().StringVar(&voiceName, "name", "", "声音显示名")
	voiceEditCmd.Flags().StringVar(&voiceProvider, "provider", "", "TTS 供应商")
	voiceEditCmd.Flags().StringVar(&voiceBlID, "voice", "", "百炼语音 ID")
	voiceEditCmd.Flags().StringVar(&voiceModel, "model", "", "驱动模型")
	voiceEditCmd.Flags().StringVar(&voiceInstruction, "instruction", "", "风格指令")
	voiceEditCmd.Flags().Float64Var(&voiceRate, "rate", 0, "语速 0.5-2.0")
	voiceEditCmd.Flags().Float64Var(&voicePitch, "pitch", 0, "音高 0.5-2.0")
	voiceEditCmd.Flags().StringVar(&voiceStyleNote, "style-note", "", "风格说明")

	voicePreviewCmd.Flags().StringVar(&voicePreviewText, "text", "", "试音文本，留空用默认")

	voiceDesignCmd.Flags().StringVar(&voiceName, "name", "", "声音显示名（必填）")
	voiceDesignCmd.Flags().StringVar(&voiceProvider, "provider", "", "TTS 供应商，默认 bailian")
	voiceDesignCmd.Flags().StringVar(&voiceBuildPrompt, "prompt", "", "声音描述（必填，如「沉稳的中年男声，书卷气，语速从容」）")
	voiceDesignCmd.Flags().StringVar(&voicePreviewText, "preview-text", "", "试听文本，留空用默认")
	voiceDesignCmd.Flags().StringVar(&voiceModel, "model", "", "驱动模型，默认 cosyvoice-v3-flash")
	voiceDesignCmd.Flags().StringVar(&voiceBuildLanguage, "language", "", "语种提示，逗号分隔，默认 zh")
	voiceDesignCmd.Flags().StringVar(&voiceStyleNote, "style-note", "", "风格说明（仅展示用）")

	voiceCloneCmd.Flags().StringVar(&voiceName, "name", "", "声音显示名（必填）")
	voiceCloneCmd.Flags().StringVar(&voiceProvider, "provider", "", "TTS 供应商，默认 bailian")
	voiceCloneCmd.Flags().StringVar(&voiceCloneAudio, "audio", "", "参考音频本地路径（内部走 bl file upload）")
	voiceCloneCmd.Flags().StringVar(&voiceCloneAudioURL, "audio-url", "", "参考音频公网/oss URL（与 --audio 二选一）")
	voiceCloneCmd.Flags().StringVar(&voiceModel, "model", "", "驱动模型，默认 cosyvoice-v3-flash")
	voiceCloneCmd.Flags().StringVar(&voiceBuildLanguage, "language", "", "语种提示，逗号分隔，默认 zh")
	voiceCloneCmd.Flags().Float64Var(&voiceCloneMaxAudio, "max-audio-length", 0, "参考音频最大时长秒 3-30，0=供应商默认")
	voiceCloneCmd.Flags().BoolVar(&voiceClonePreprocess, "preprocess", false, "参考音频预处理（降噪/增强）")
	voiceCloneCmd.Flags().StringVar(&voiceStyleNote, "style-note", "", "风格说明（仅展示用）")

	voiceCmd.AddCommand(voiceListCmd, voiceShowCmd, voiceAddCmd, voiceEditCmd, voiceRmCmd, voicePreviewCmd, voiceDesignCmd, voiceCloneCmd)
	rootCmd.AddCommand(voiceCmd)
}
