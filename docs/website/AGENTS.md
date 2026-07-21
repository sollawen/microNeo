# MkDocs 使用说明（给 AI 参考）

本项目用 **MkDocs + Material 主题 + i18n** 做文档站。配置在 `mkdocs.yml`。

## 启动本地预览

```bash
mkdocs serve --livereload --dirty -a 127.0.0.1:8000
```

- `--livereload`：**必须加**。mkdocs 1.6.1 + click 8.3 兼容性 bug，不加这个参数 observer 不会启动，改文件无反应。
- `--dirty`：增量重建，只 build 改动的页面。日常开发用这个。
- `-a 127.0.0.1:8000`：监听地址和端口。

启动后浏览器打开 `http://127.0.0.1:8000/microNeo/zh/`。

## rebuild 模式

| 模式 | 命令 | 语义 | 用途 |
|---|---|---|---|
| 全量 | `mkdocs serve --livereload` | 每次改动重建**所有**页面 | 默认 |
| 增量 | `mkdocs serve --livereload --dirty` | 只重建改动的页面 | **推荐日常用** |

`--dirty` 的局限：改导航/侧边栏标题/`mkdocs.yml` 时其他页面不更新，需重启服务。

## 几个常用命令

```bash
# 启动预览（推荐）
mkdocs serve --livereload --dirty -a 127.0.0.1:8000

# 全量构建到 site/ 目录（部署用）
mkdocs build

# 看实时日志
tail -f /tmp/mkdocs.log

# 停止服务
pkill -f "mkdocs serve"
```

## 注意事项

- 监听机制：mkdocs 用轮询（PollingObserver，0.5 秒一次），**不是** fsevents。这是 mkdocs 源码写死的，没有开关切换。对本项目完全够用。
- 文件改动后约 0.5 秒检测到，再 0.3 秒重建完成。
- `--dirty` 模式改了 `mkdocs.yml` / nav / 新增页面时，要重启服务才生效。
- HTML 容器里放图片必须用 markdown `![alt](src)` 语法（mkdocs 会按 build 深度自动补 `../`），不能用 `<img src="...">`（src 不会被翻译，i18n 下英文版会 404）。示例：`<div class="image-pair" markdown>![a](../assets/a.png)![b](../assets/b.png)</div>`。markdown 在 div 里会被 `<p>` 包裹，flex 容器并排时给 `<p>` 加 `display: contents` 让它不参与布局

## GitHub README 视频：必须用 .mov

GitHub README 用 `<img src="./repo_file">` 渲染视频，**认 `.mov` 后缀，与 codec 无关**。mp4 不行，mov 行（H.264 / H.265 都行）。

从源 mp4 remux 出 mov（不重编码）：

```bash
ffmpeg -i xxx.mp4 -c copy -tag:v hvc1 -f mov xxx.mov
```

本项目：`assets/aibp-opencode-hevc.mov`（README 用）/ `aibp-opencode_h265_x265_crf28.mp4`（网站 `<video>` 用）。换新演示时两个一起更新。
