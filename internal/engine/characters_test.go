package engine

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sjzsdu/story/internal/domain"
)

// imageRefTestRef 校验一条 RefImage 引用（§23 第二步：视觉参考收编为 image_ref 素材）：
//   - "asset:<id>" 引用 → 查素材行，断言 kind=image_ref、素材文件存在非空且位于
//     素材库目录 data/assets/image_ref/ 内（防路径穿越）；
//   - 字面路径（登记失败回退 / 历史数据）→ 断言位于 refs 目录内且文件有效；
//   - 两种形态都断言原位文件 refs/<slug>.png 仍在——幂等跳过靠原位文件判定，
//     无论引用形态如何都不能丢。
func imageRefTestRef(t *testing.T, f *fixture, ref, wantDir, name string) {
	t.Helper()
	if domain.IsAssetRef(ref) {
		as, err := f.repo.GetAsset(context.Background(), domain.AssetIDFromRef(ref))
		if err != nil || as == nil {
			t.Fatalf("素材引用 %s 无法解析: %v", ref, err)
		}
		if as.Kind != domain.AssetKindImageRef {
			t.Fatalf("素材 kind = %s, 期望 image_ref: %+v", as.Kind, as)
		}
		assetDir := filepath.Join(filepath.Dir(f.eng.projectsDir), "assets", domain.AssetKindImageRef)
		if !strings.Contains(as.Path, assetDir) {
			t.Fatalf("素材副本应位于素材库目录 %s: %s", assetDir, as.Path)
		}
		if fi, err := os.Stat(as.Path); err != nil || fi.Size() == 0 {
			t.Fatalf("素材文件不存在或为空: %s (%v)", as.Path, err)
		}
	} else {
		if !filepath.IsAbs(ref) {
			t.Fatalf("字面路径引用应为绝对路径: %s", ref)
		}
		if !strings.Contains(ref, wantDir) {
			t.Fatalf("引用应位于 refs 目录 %s: %s", wantDir, ref)
		}
		if fi, err := os.Stat(ref); err != nil || fi.Size() == 0 {
			t.Fatalf("引用文件不存在或为空: %s (%v)", ref, err)
		}
	}
	orig := filepath.Join(wantDir, slugFileName(name)+".png")
	if fi, err := os.Stat(orig); err != nil || fi.Size() == 0 {
		t.Fatalf("原位参考图不存在或为空: %s (%v)", orig, err)
	}
}

// charRefImages 提取人物设定集的 RefImage 引用串列表（顺序稳定），供关联稳定性断言。
func charRefImages(chars []domain.CharacterSetting) []string {
	out := make([]string, 0, len(chars))
	for _, c := range chars {
		out = append(out, c.RefImage)
	}
	return out
}

func sampleCharacters() []domain.CharacterSetting {
	return []domain.CharacterSetting{
		{Name: "张仪", Identity: "秦国相国", Appearance: "约四旬，清瘦挺拔，深青色深衣束发戴冠", Temperament: "沉毅多智"},
		{Name: "苏秦", Identity: "六国纵约长", Appearance: "三十许，面容清癯，皂色儒服", Temperament: "坚韧执拗"},
	}
}

func setCharacters(t *testing.T, f *fixture, cs []domain.CharacterSetting) {
	t.Helper()
	se, err := f.repo.GetSeries(context.Background(), "guiguzi")
	if err != nil {
		t.Fatal(err)
	}
	se.Characters = cs
	se.UpdatedAt = time.Now()
	if err := f.repo.UpdateSeries(context.Background(), se); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateSeriesKeyframes(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	setCharacters(t, f, sampleCharacters())

	cs, err := f.eng.GenerateSeriesKeyframes(ctx, "guiguzi", false)
	if err != nil {
		t.Fatalf("生成定妆照: %v", err)
	}
	if len(cs) != 2 {
		t.Fatalf("人物数 = %d", len(cs))
	}
	wantDir := filepath.Join(f.eng.projectsDir, "guiguzi", RefsDirName)
	for _, ch := range cs {
		if ch.RefImage == "" {
			t.Fatalf("%s 的 RefImage 未回填", ch.Name)
		}
		imageRefTestRef(t, f, ch.RefImage, wantDir, ch.Name)
	}
	if calls := f.images.CallsCount(); calls != 2 {
		t.Fatalf("图片调用次数 = %d, 期望 2", calls)
	}

	// 幂等：已存在则跳过。
	if _, err := f.eng.GenerateSeriesKeyframes(ctx, "guiguzi", false); err != nil {
		t.Fatal(err)
	}
	if calls := f.images.CallsCount(); calls != 2 {
		t.Fatalf("幂等复跑不应再次调用图片生成, 调用次数 = %d", calls)
	}
	seBefore, err := f.repo.GetSeries(ctx, "guiguzi")
	if err != nil {
		t.Fatal(err)
	}
	beforeRefs := charRefImages(seBefore.Characters)

	// force=true 重新生成。
	if _, err := f.eng.GenerateSeriesKeyframes(ctx, "guiguzi", true); err != nil {
		t.Fatal(err)
	}
	if calls := f.images.CallsCount(); calls != 4 {
		t.Fatalf("force 复跑应重新生成, 调用次数 = %d", calls)
	}

	// 已持久化回系列；force 重生后引用必须保持同一素材行（防孤儿 image_ref 素材，
	// adoptImageRefAsset 路 1 复用原 id）。
	se, _ := f.repo.GetSeries(ctx, "guiguzi")
	if after := charRefImages(se.Characters); !reflect.DeepEqual(beforeRefs, after) {
		t.Fatalf("force 重生改变了素材行关联:\n before=%v\n after=%v", beforeRefs, after)
	}
	for _, ch := range se.Characters {
		if ch.RefImage == "" {
			t.Fatalf("系列中的 %s 未持久化 RefImage", ch.Name)
		}
		imageRefTestRef(t, f, ch.RefImage, wantDir, ch.Name)
	}
}

func TestGenerateSeriesKeyframesNoCharacters(t *testing.T) {
	f := setup(t)
	if _, err := f.eng.GenerateSeriesKeyframes(context.Background(), "guiguzi", false); err == nil {
		t.Fatal("无人物设定时应报错")
	}
}

func sampleVisualRefs() []domain.VisualRef {
	return []domain.VisualRef{
		{Kind: domain.RefKindCharacter, Name: "张仪", Description: "约四旬，清瘦挺拔，深青色深衣束发戴冠"},
		{Kind: domain.RefKindCharacter, Name: "苏秦", Description: "三十许，面容清癯，皂色儒服"},
	}
}

func TestRefImagesForScene(t *testing.T) {
	refs := sampleVisualRefs()
	refs[0].RefImage = "/tmp/refs/张仪.png"

	if got := refImagesForScene("张仪立于殿前，苏秦在侧", refs); len(got) != 1 || got[0] != refs[0].RefImage {
		t.Fatalf("应只匹配张仪: %v", got)
	}
	if got := refImagesForScene("空旷市集，行人往来", refs); got != nil {
		t.Fatalf("无人物时不应有参考图: %v", got)
	}
	// 未生成参考图的人物不参与匹配。
	if got := refImagesForScene("苏秦伏案疾书", refs); got != nil {
		t.Fatalf("RefImage 为空不应匹配: %v", got)
	}
	// 场景参考同样按名称匹配。
	scene := domain.VisualRef{Kind: domain.RefKindScene, Name: "兰若寺大殿", Description: "破败古寺", RefImage: "/tmp/refs/lanruo.png"}
	if got := refImagesForScene("夜色中的兰若寺大殿", append(refs, scene)); len(got) != 1 || got[0] != scene.RefImage {
		t.Fatalf("应匹配场景参考图: %v", got)
	}
}

func TestRefPromptPrefix(t *testing.T) {
	refs := sampleVisualRefs()
	refs[0].RefImage = "/tmp/refs/张仪.png"
	refs[1].RefImage = "/tmp/refs/苏秦.png"
	refs = append(refs, domain.VisualRef{Kind: domain.RefKindScene, Name: "兰若寺大殿", RefImage: "/tmp/refs/lanruo.png"})

	imgs := []string{refs[1].RefImage, refs[0].RefImage, "/tmp/refs/lanruo.png"}
	prefix := refPromptPrefix(refs, imgs)
	if !strings.Contains(prefix, "Image 1 为人物苏秦") || !strings.Contains(prefix, "Image 2 为人物张仪") {
		t.Fatalf("前缀应按 imgs 顺序声明人物: %s", prefix)
	}
	if !strings.Contains(prefix, "Image 3 为场景「兰若寺大殿」") {
		t.Fatalf("前缀应声明场景: %s", prefix)
	}
	if refPromptPrefix(refs, nil) != "" {
		t.Fatal("无参考图时前缀应为空")
	}
}

func TestMergeVisualRefs(t *testing.T) {
	se := &domain.Series{Characters: sampleCharacters()}
	epRefs := []domain.VisualRef{
		// 同名人物集级覆盖系列级（本集形象可重定义）。
		{Kind: domain.RefKindCharacter, Name: "张仪", Description: "本集专属张仪形象", RefImage: "/tmp/ep/zhangyi.png"},
		// 本集新人物。
		{Kind: domain.RefKindCharacter, Name: "聂小倩", Description: "素白襦裙"},
		// 本集场景。
		{Kind: domain.RefKindScene, Name: "兰若寺大殿", Description: "破败古寺"},
	}
	merged := mergeVisualRefs(se, epRefs)
	if len(merged) != 4 { // 苏秦（系列）+ 张仪（集级覆盖）+ 聂小倩 + 兰若寺
		t.Fatalf("合并后条数 = %d, 期望 4: %+v", len(merged), merged)
	}
	for _, r := range merged {
		if r.Name == "张仪" {
			if r.Description != "本集专属张仪形象" || r.RefImage != "/tmp/ep/zhangyi.png" {
				t.Fatalf("集级同名参考应覆盖系列级: %+v", r)
			}
		}
		if r.Name == "苏秦" && r.RefImage == "" {
			// 苏秦系列级无图，仅确认来自系列。
		}
	}
}

func TestPreserveRefImages(t *testing.T) {
	old := []domain.VisualRef{
		{Kind: domain.RefKindCharacter, Name: "聂小倩", Description: "旧描述", RefImage: "/tmp/refs/nie.png"},
		{Kind: domain.RefKindScene, Name: "兰若寺大殿", Description: "旧场景", RefImage: "/tmp/refs/temple.png"},
		{Kind: domain.RefKindCharacter, Name: "将被删除的人物", RefImage: "/tmp/refs/gone.png"},
	}
	fresh := []domain.VisualRef{
		{Kind: domain.RefKindCharacter, Name: "聂小倩", Description: "新描述"},
		{Kind: domain.RefKindScene, Name: "兰若寺大殿", Description: "新场景描述", RefImage: "/tmp/refs/forced.png"},
		{Kind: domain.RefKindCharacter, Name: "新增人物", Description: "新增描述"},
		{Name: "  "}, // 空名丢弃
	}
	out := preserveRefImages(old, fresh)
	if len(out) != 3 {
		t.Fatalf("应丢弃空名, 得到 %d 条", len(out))
	}
	byName := map[string]domain.VisualRef{}
	for _, r := range out {
		byName[r.Name] = r
	}
	if byName["聂小倩"].RefImage != "/tmp/refs/nie.png" || byName["聂小倩"].Description != "新描述" {
		t.Fatalf("应延续旧图、采用新描述: %+v", byName["聂小倩"])
	}
	if byName["兰若寺大殿"].RefImage != "/tmp/refs/forced.png" {
		t.Fatalf("新结果自带图时不应被覆盖: %+v", byName["兰若寺大殿"])
	}
	if byName["新增人物"].RefImage != "" {
		t.Fatal("新参考不应凭空有图")
	}
}

func TestGenerateEpisodeRefs(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	ep, err := f.repo.GetEpisode(ctx, f.epID)
	if err != nil {
		t.Fatal(err)
	}
	ep.Refs = []domain.VisualRef{
		{Kind: domain.RefKindCharacter, Name: "聂小倩", Description: "约十八九岁，素白襦裙外罩浅青纱衫"},
		{Kind: domain.RefKindScene, Name: "兰若寺大殿", Description: "破败古寺大殿，冷青色月光"},
	}
	if err := f.repo.SaveEpisode(ctx, ep); err != nil {
		t.Fatal(err)
	}

	refs, err := f.eng.GenerateEpisodeRefs(ctx, f.epID, false)
	if err != nil {
		t.Fatalf("生成本集视觉参考图: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("参考数 = %d", len(refs))
	}
	wantDir := filepath.Join(ep.WorkDir, EpisodeRefsDirName)
	for _, r := range refs {
		if r.RefImage == "" {
			t.Fatalf("%s 的 RefImage 未回填", r.Name)
		}
		imageRefTestRef(t, f, r.RefImage, wantDir, r.Name)
	}
	if calls := f.images.CallsCount(); calls != 2 {
		t.Fatalf("图片调用次数 = %d, 期望 2", calls)
	}

	// 幂等复跑跳过。
	if _, err := f.eng.GenerateEpisodeRefs(ctx, f.epID, false); err != nil {
		t.Fatal(err)
	}
	if calls := f.images.CallsCount(); calls != 2 {
		t.Fatalf("幂等复跑不应再次出图, 调用次数 = %d", calls)
	}

	// 已持久化回集；幂等复跑后引用仍可解析（asset: 与字面路径都有效）。
	got, _ := f.repo.GetEpisode(ctx, f.epID)
	for _, r := range got.Refs {
		if r.RefImage == "" {
			t.Fatalf("集中的 %s 未持久化 RefImage", r.Name)
		}
		imageRefTestRef(t, f, r.RefImage, wantDir, r.Name)
	}
}

func TestGenerateEpisodeRefsEmpty(t *testing.T) {
	f := setup(t)
	if _, err := f.eng.GenerateEpisodeRefs(context.Background(), f.epID, false); err == nil {
		t.Fatal("无视觉参考时应报错")
	}
}

func TestProduceUsesRefImages(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	cs := sampleCharacters()
	cs[0].RefImage = filepath.Join(f.eng.projectsDir, "guiguzi", RefsDirName, "张仪.png")
	setCharacters(t, f, cs)

	// 覆盖分镜样本：场景 2 出现张仪，其余不含设定集人物。
	f.boards.Storyboard = &domain.Storyboard{Scenes: []domain.Scene{
		{ID: 1, VisualPrompt: "战国书房，竹简与青铜灯", Narration: "第一句", DurationSec: 4, Camera: "远景"},
		{ID: 2, VisualPrompt: "张仪（约四旬，清瘦挺拔，深青色深衣束发戴冠）立于殿前", Narration: "第二句", DurationSec: 4, Camera: "近景"},
		{ID: 3, VisualPrompt: "列国宫殿，夯土高台", Narration: "第三句", DurationSec: 4, Camera: "推镜"},
		{ID: 4, VisualPrompt: "苏秦伏案疾书，皂色儒服", Narration: "第四句", DurationSec: 4, Camera: "中景"},
	}}

	if _, err := f.eng.GenerateStory(ctx, f.epID, DeriveOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.eng.PlanStoryboard(ctx, f.epID, DeriveOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.eng.Produce(ctx, f.epID, DeriveOptions{}); err != nil {
		t.Fatal(err)
	}

	// 分镜画面里出现「张仪」的镜头应携带参考图（走 video ref）。
	ep, _ := f.repo.GetEpisode(ctx, f.epID)
	media := ep.ActiveNodeOfStage(domain.StageMedia)
	board := ep.ActiveNodeOfStage(domain.StageStoryboard)
	matched := 0
	for _, clip := range media.Clips {
		if clip.Path == "" {
			continue
		}
		idx := clip.SceneID - 1
		if idx < 0 || idx >= len(board.Storyboard.Scenes) {
			continue
		}
		if strings.Contains(board.Storyboard.Scenes[idx].VisualPrompt, "张仪") {
			matched++
		}
	}
	refCalls := 0
	for _, req := range f.videos.Requests {
		if len(req.RefImages) > 0 {
			refCalls++
			if req.RefImages[0] != cs[0].RefImage {
				t.Fatalf("参考图路径不符: %s", req.RefImages[0])
			}
		}
	}
	if refCalls != matched {
		t.Fatalf("携带参考图的镜头数 = %d, 期望 %d", refCalls, matched)
	}
}

func TestCharacterLines(t *testing.T) {
	lines := characterLines(sampleCharacters())
	if len(lines) != 2 {
		t.Fatalf("行数 = %d", len(lines))
	}
	if !strings.HasPrefix(lines[0], "张仪（秦国相国）：约四旬") {
		t.Fatalf("格式不符: %s", lines[0])
	}
	if !strings.Contains(lines[0], "气质：沉毅多智") {
		t.Fatalf("应含气质: %s", lines[0])
	}
	if characterLines(nil) != nil {
		t.Fatal("空设定应返回 nil")
	}
}
