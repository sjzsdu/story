// Package templates 集中管理朝代视觉锚定资料与 LLM 系统提示词。
package templates

// DynastyPack 单个朝代/时期的视觉约束。
type DynastyPack struct {
	// Keyword 匹配朝代关键词（包含匹配）。
	Keywords []string
	// Clothing 服饰特征。
	Clothing string
	// Architecture 建筑/室内特征。
	Architecture string
	// Objects 典型器物与道具。
	Objects string
	// Color 色调倾向。
	Color string
	// Customs 起居方式与习俗要点。
	Customs string
}

// DynastyPacks 按时间顺序排列的朝代视觉资料。
// 分镜生成时据此锚定画面，避免出现时代错置（穿越道具/服饰/建筑）。
var DynastyPacks = []DynastyPack{
	{
		Keywords:     []string{"先秦", "夏", "商", "周", "春秋", "战国", "秦"},
		Clothing:     "贵族着深衣、曲裾，麻布葛布面料，平民短褐束腿；无纽扣，以带钩束腰；头裹巾帻或戴冠",
		Architecture: "茅草屋顶、夯土台基、木构柱廊；室内席地而坐，低矮案几，青铜灯具；城池为夯土城墙",
		Objects:      "青铜器（鼎、爵、尊）、竹简木牍、陶豆、戈矛剑戟、马车、编钟；无纸、无瓷器、无桌椅",
		Color:        "秦尚黑，玄黑与赭红为主色；战国诸侯服饰色彩偏沉暗",
		Customs:      "席地跪坐（跽坐），食器为青铜或漆器，书写用刀刻简牍或毛笔书于简帛",
	},
	{
		Keywords:     []string{"汉", "西汉", "东汉", "楚汉"},
		Clothing:     "男子曲裾/直裾深衣，女子襦裙，官员进贤冠、武人鹖冠；丝麻面料，宽袍大袖",
		Architecture: "高台建筑、阙楼、夯土墙配瓦顶；室内帷幔屏风，席地而坐，后期出现低榻",
		Objects:      "漆器耳杯、陶制明器、铜灯、简牍帛书、纸张罕见（东汉末始有）、铁制农具环首刀",
		Color:        "汉初尚黑红，后期朱红、玄色、土黄；墓葬帛画用色浓艳",
		Customs:      "分餐跪坐而食，竹简书写，车马出行，门阀之前立阙",
	},
	{
		Keywords:     []string{"三国", "魏晋", "南北朝", "两晋"},
		Clothing:     "男子宽衫大袖、褒衣博带，名士扪虱清谈、袒胸露臂；女子杂裾垂髾服、步摇簪钗",
		Architecture: "坞堡、门阀庄园、佛窟与木塔初兴；室内坐榻、隐囊、曲足案",
		Objects:      "青瓷成熟、麈尾、凭几、多足砚、卷轴书帖、马镫普及",
		Color:        "崇尚素雅玄淡，白衣、青灰、石青；佛教艺术带西域金碧色",
		Customs:      "清谈、服药饮酒、席地与坐榻并存；胡汉文化交融，高足家具开始传入",
	},
	{
		Keywords:     []string{"隋", "唐", "盛唐", "晚唐", "武则天"},
		Clothing:     "男子圆领袍、幞头、长靿靴；女子齐胸襦裙、披帛、半臂，丰腴为美，面饰花钿",
		Architecture: "长安城棋盘式坊市、庑殿顶斗拱宏大、朱柱白墙；室内出现高桌、腰鼓形凳、胡床",
		Objects:      "唐三彩、金银器、越窑青瓷邢窑白瓷、卷轴书画、雕版印刷、西域玻璃器",
		Color:        "朱红、明黄、石绿、鎏金，浓丽华贵，敦煌壁画配色",
		Customs:      "坊市制、胡风盛行（胡服胡乐葡萄酒），垂足而坐逐渐普及，女性可着男装、骑马出行",
	},
	{
		Keywords:     []string{"宋", "北宋", "南宋", "两宋"},
		Clothing:     "男子交领圆领襕衫、直裰，女子瘦长背子、抹胸襦裙；色彩清雅，纹样素净",
		Architecture: "街市临街开店（坊市制瓦解）、悬鱼博风、白墙黛瓦园林草堂；高足桌椅普及",
		Objects:      "五大名窑瓷器（汝官哥钧定）、雕版书册、交子纸币、活字印刷、市井招牌幌子",
		Color:        "天青、月白、雅灰、木色，极简含蓄",
		Customs:      "垂足高坐、点茶斗茶、瓦舍勾栏看戏、士子书院讲学、市民商业生活发达",
	},
	{
		Keywords:     []string{"元", "元代", "蒙古"},
		Clothing:     "蒙古族质孙服、辫线袄、瓦楞帽；汉人着唐宋式交领衣，色彩偏暗；女子襦裙加半臂",
		Architecture: "大都城、藏式佛塔（白塔）、穹顶毡帐与中原木构并存；民居沿用金宋样式",
		Objects:      "青花瓷成熟、釉里红、蒙古弯刀、马术器具、杂剧道具、算盘",
		Color:        "钴蓝、白、银灰，青花蓝白为主视觉符号",
		Customs:      "多民族杂处，杂剧散曲兴盛，驿站体系发达；南方汉人士绅生活延续宋风",
	},
	{
		Keywords:     []string{"明", "明代", "朱元璋", "崇祯"},
		Clothing:     "男子交领长袍或圆领补服、乌纱帽；官员飞鱼服/蟒袍赐服；女子立领比甲、马面裙、凤冠霞帔",
		Architecture: "紫禁城官式建筑、江南私家园林、砖雕门楼；室内拔步床、条案、圈椅（明式家具）",
		Objects:      "青花瓷巅峰、斗彩、宣德炉、折扇、线装书、八股闱墨、火器火铳",
		Color:        "朱红、明黄、青花蓝、髹栗色家具，典重端庄",
		Customs:      "科举八股、乡约宗族、茶馆酒肆，社会等级在服色房舍上规制严格",
	},
	{
		Keywords:     []string{"清", "清代", "清朝", "康熙", "乾隆", "晚清"},
		Clothing:     "男子剃发留辫、长袍马褂、马蹄袖、顶戴花翎；女子旗装（晚清发展为旗袍）、花盆底鞋；汉族女性仍着袄裙",
		Architecture: "四合院、宫廷硬山歇山黄琉璃瓦、苏州园林；晚清出现洋楼、玻璃窗、照相馆",
		Objects:      "粉彩珐琅彩瓷器、鼻烟壶、自鸣钟、鸦片烟枪（晚清）、洋枪洋炮、煤油灯、电报",
		Color:        "明黄（皇家）、石青、绛紫；晚清色调加入西洋颜料的艳色",
		Customs:      "早中期旗汉分治，晚清开埠后新旧杂陈——火车、电报、西装与辫子并存",
	},
}

// FallbackPack 无法判定朝代时使用的通用古风约束。
var FallbackPack = DynastyPack{
	Clothing:     "交领右衽汉服、宽袍束带，避免任何现代与清代符号（无辫子、无立领盘扣）",
	Architecture: "木构殿堂、夯土或砖石台基、筒瓦屋顶、纸棂木窗",
	Objects:      "竹简或卷轴、青铜/陶瓷器皿、油灯烛火、马车；严禁出现玻璃、塑料制品、电线、现代金属构件",
	Color:        "沉稳的大地色、青灰、赭石、旧绢色",
	Customs:      "中式古礼起居，作揖行礼",
}

// MatchDynasty 根据朝代描述返回视觉约束包。
func MatchDynasty(dynasty string) DynastyPack {
	for _, p := range DynastyPacks {
		for _, kw := range p.Keywords {
			if contains(dynasty, kw) {
				return p
			}
		}
	}
	return FallbackPack
}

func contains(s, sub string) bool {
	if len(sub) == 0 || len(s) < len(sub) {
		return len(sub) == 0
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// VisualAnchor 把朝代约束渲染成可直接拼进 prompt 的文字。
func (p DynastyPack) VisualAnchor() string {
	s := "【时代视觉要求·必须严格遵守】\n"
	s += "- 服饰：" + p.Clothing + "\n"
	s += "- 建筑环境：" + p.Architecture + "\n"
	s += "- 器物道具：" + p.Objects + "\n"
	s += "- 色调：" + p.Color + "\n"
	s += "- 起居习俗：" + p.Customs + "\n"
	s += "- 严禁出现任何时代错置元素（现代服饰、眼镜、拉链、混凝土、电线、塑料制品等）"
	return s
}
