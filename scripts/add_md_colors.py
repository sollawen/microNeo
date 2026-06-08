#!/usr/bin/env python3
"""
为每个 runtime/colorschemes/*.micro 追加 md-* 颜色定义。
策略：复用各主题已有的颜色组，保持风格一致。
"""
import os
import re
import sys

COLORSCHEMES_DIR = os.path.join(os.path.dirname(__file__), '..', 'runtime', 'colorschemes')

# md-* 映射规则：每个 md-* 条目引用一个已有的颜色组名
# 框架装饰色需要特殊处理（提取灰色）
MD_COLOR_MAP = {
    'md-header':         'special',        # 标题 - 醒目色
    'md-hr':             'special',        # 分割线 - 同标题
    'md-blockquote':     'preproc',        # 引用 - preproc
    'md-bold':           'constant.string',# 粗体 - 字符串色（通常醒目但不刺眼）
    'md-italic':         'default',        # 斜体 - 默认色
    'md-strikethrough':  'default',        # 删除线 - 默认色
    'md-inline-code':    'statement',      # 行内代码 - statement（醒目）
    'md-list':           'statement',      # 列表 - statement
    'md-checkbox':       'statement',      # 复选框 - statement
    'md-link':           'constant',       # 链接 - constant
    'md-url':            'constant',       # URL - constant
    'md-image':          'constant',       # 图片 - constant
    'md-codeblock':      'default',        # 代码块文字 - 默认色
    'md-misc':           'preproc',        # 特殊符号 - preproc
}

def extract_bg(default_line):
    """从 default 颜色行提取背景色"""
    if not default_line:
        return ''
    # 格式: color-link default "fg,bg" 或 "fg" 或 fg,bg（无引号）
    m = re.search(r'color-link default\s+"?([^"]+)"?', default_line)
    if not m:
        return ''
    val = m.group(1).strip()
    if ',' in val:
        return val.split(',')[1].strip()
    return ''

def extract_color_line(content, group):
    """从主题内容中提取某个颜色组的完整行"""
    for line in content.split('\n'):
        if re.match(rf'color-link {re.escape(group)}\s', line):
            return line.strip()
    return ''

def extract_fg(color_line):
    """从颜色行提取前景色"""
    if not color_line:
        return ''
    m = re.search(r'color-link \S+\s+"?([^"]+)"?', color_line)
    if not m:
        return ''
    val = m.group(1).strip()
    if ',' in val:
        return val.split(',')[0].strip()
    return val

def make_color_ref(group, color_line, bg):
    """生成颜色引用字符串，如 "#A6E22E,#282828" """
    fg = extract_fg(color_line)
    if not fg:
        return ''
    if bg:
        return f'{fg},{bg}'
    return fg

def is_light_bg(bg):
    """粗略判断背景是否为浅色"""
    if not bg:
        return False
    # 去掉 # 号
    hex_bg = bg.lstrip('#')
    if len(hex_bg) == 6:
        try:
            r, g, b = int(hex_bg[0:2], 16), int(hex_bg[2:4], 16), int(hex_bg[4:6], 16)
            luminance = (0.299 * r + 0.587 * g + 0.114 * b) / 255
            return luminance > 0.5
        except ValueError:
            pass
    # 256色号粗略判断
    try:
        n = int(bg)
        return n > 200 or n in [230, 231, 255, 254, 253]
    except ValueError:
        pass
    # 颜色名
    if bg in ['white', 'white_smoke']:
        return True
    return False

def process_file(filepath):
    """处理单个 colorscheme 文件"""
    filename = os.path.basename(filepath)
    name = filename.replace('.micro', '')
    
    with open(filepath, 'r') as f:
        content = f.read()
    
    # 如果已有 md-* 定义，跳过
    if 'md-header' in content:
        print(f'  SKIP (already has md-*) {filename}')
        return
    
    # default.micro 使用 include，特殊处理
    if name == 'default':
        print(f'  SKIP (uses include) {filename}')
        return
    
    lines = content.rstrip('\n').split('\n')
    
    # 提取背景色
    default_line = extract_color_line(content, 'default')
    bg = extract_bg(default_line)
    light = is_light_bg(bg)
    
    # 框架色：根据主题明暗选择灰色
    if light:
        frame_fg = '#B0B0B0'
        label_fg = '#808080'
    else:
        frame_fg = '#505050'
        label_fg = '#909090'
    
    # inline-code 背景色（稍暗于默认背景）
    # 简化处理：只用前景色，不加背景
    inline_code_line = extract_color_line(content, 'statement')
    
    # 生成 md-* 行
    md_lines = ['\n# Markdown 专用']
    
    for md_name, group in MD_COLOR_MAP.items():
        color_line = extract_color_line(content, group)
        color_ref = make_color_ref(group, color_line, bg)
        if color_ref:
            md_lines.append(f'color-link {md_name} "{color_ref}"')
        else:
            # 没有该颜色组，用 default
            md_lines.append(f'color-link {md_name} "default"')
    
    # md-frame 和 md-frame-label 需要特殊处理（不在 MD_COLOR_MAP 里）
    if bg:
        md_lines.append(f'color-link md-frame "{frame_fg},{bg}"')
        md_lines.append(f'color-link md-frame-label "{label_fg},{bg}"')
    else:
        md_lines.append(f'color-link md-frame "{frame_fg}"')
        md_lines.append(f'color-link md-frame-label "{label_fg}"')
    
    # 追加到文件
    new_content = content.rstrip('\n') + '\n' + '\n'.join(md_lines) + '\n'
    
    with open(filepath, 'w') as f:
        f.write(new_content)
    
    print(f'  DONE {filename} (bg={bg or "none"}, light={light})')

def main():
    print('Adding md-* colors to runtime colorschemes...')
    
    for filename in sorted(os.listdir(COLORSCHEMES_DIR)):
        if filename.endswith('.micro'):
            filepath = os.path.join(COLORSCHEMES_DIR, filename)
            process_file(filepath)
    
    print('\nDone!')

if __name__ == '__main__':
    main()
