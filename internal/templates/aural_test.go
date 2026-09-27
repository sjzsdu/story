package templates

import "testing"

// 开场/结尾反例直接取自后汉书 e01 第一版漏网稿（§27 重构的导火索），
// 作为回归用例：这些写法必须永远判废。
func TestCheckStoryAural_BackgroundSlideOpening(t *testing.T) {
	// 第一句是钩子，第二句滑进编年史——最高频失败模式，必须判废。
	content := "《后汉书》给刘秀的第一个画面，是新野谷市旁的一辆车。地皇三年，南阳饥荒。王莽的天下已经连年受灾。"
	if err := CheckStoryAural("谷车", content); err == nil {
		t.Fatalf("编年纪年开场必须判废:\n%s", content)
	}
}

func TestCheckStoryAural_BioOpening(t *testing.T) {
	for _, content := range []string{
		"刘秀是南阳蔡阳人。他年轻时勤于田亩。",
		"刘秀，南阳蔡阳人。他年轻时勤于田亩。",
		"刘秀生于南阳蔡阳。他年轻时勤于田亩。",
	} {
		if err := CheckStoryAural("谷车", content); err == nil {
			t.Fatalf("生平档案开场必须判废:\n%s", content)
		}
	}
}

func TestCheckStoryAural_UnrelatedClosing(t *testing.T) {
	content := "他二十八岁。他买下一张弩，翻过来看了看弩臂，扣了一次机括。几个月后，王莽将领甄阜死在南阳。"
	if err := CheckStoryAural("谷车", content); err == nil {
		t.Fatalf("无关第三者死亡句收尾必须判废:\n%s", content)
	}
	// 编年纪年收尾
	content = "他把弩抱回了家。建安十三年。"
	if err := CheckStoryAural("谷车", content); err == nil {
		t.Fatalf("编年纪年收尾必须判废:\n%s", content)
	}
}

func TestCheckStoryAural_GoodOpeningsPass(t *testing.T) {
	for _, content := range []string{
		// 相对时间开场放行
		"那一年，他做了一件让全族捏一把汗的事。他把卖谷的钱全买了弓弩。",
		// 正常钩子开场
		"张仪被人按在地上，打了几百板子，打得浑身是血。他抬起头，问妻子的第一句话不是喊疼。",
		// 钩子句里带数字年份（如「公元前 202 年」前有主体）不误杀
		"垓下的火光烧红了半边天。项羽的身边只剩下二十八骑。",
	} {
		if err := CheckStoryAural("测试", content); err != nil {
			t.Fatalf("好开场被误杀:\n%s\n错误: %v", content, err)
		}
	}
}

func TestCheckStoryAural_GoodClosingsPass(t *testing.T) {
	for _, content := range []string{
		// 主角相关死亡句（含人称）放行
		"他到死也没等来那道赦令。史书没记他最后说了什么。",
		// 留白式结尾
		"曲终，他抬眼说了句什么，史书没记。刽子手后来说，他一辈子没听过那种声音。",
		// 钩子式结尾
		"可多年之后，有人在一座荒寺里，又听见了这支曲子——那是另一个故事了。",
	} {
		if err := CheckStoryAural("测试", content); err != nil {
			t.Fatalf("好结尾被误杀:\n%s\n错误: %v", content, err)
		}
	}
}

func TestCheckStoryAural_RelativeTimeNotFlagged(t *testing.T) {
	for _, clause := range []string{"那一年", "这一年", "次年", "头一年", "那几年", "过去一年"} {
		if isChronicleClause(clause) {
			t.Fatalf("相对时间表述 %q 不应判为编年纪年", clause)
		}
	}
	for _, clause := range []string{"地皇三年", "建安十三年", "汉高祖十二年", "公元前202年"} {
		if !isChronicleClause(clause) {
			t.Fatalf("编年纪年 %q 应被判为编年纪年", clause)
		}
	}
}
