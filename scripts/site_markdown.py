"""Small Markdown renderer for the Game Facts subset used by the public site.

Supported: ATX headings, paragraphs, bullet and numbered lists, pipe tables, fenced
code, block quotes, inline code, bold, italic and links. Everything else is escaped
as plain text. The renderer is deliberately tiny; extend it only when a published
topic needs more, and keep its output free of raw HTML from the source.
"""

import html
import re
from collections.abc import Callable

type LinkResolver = Callable[[str], str]

INLINE = re.compile(
    r"`(?P<code>[^`]+)`"
    r"|\*\*(?P<bold>.+?)\*\*"
    r"|(?<![\w*])\*(?!\s)(?P<italic>[^*]+?)(?<!\s)\*(?!\w)"
    r"|\[(?P<label>[^\]]+)\]\((?P<target>[^)\s]+)\)"
)
HEADING = re.compile(r"(#{1,6})\s+(.*?)\s*#*\s*$")
BULLET = re.compile(r"[-*]\s+(.*)")
NUMBERED = re.compile(r"\d+\.\s+(.*)")
SEPARATOR_CELL = re.compile(r":?-{3,}:?")


def slugify(text: str) -> str:
    """Build a GitHub-style heading slug from rendered or raw text."""
    plain = re.sub(r"[`*]", "", text).lower()
    return re.sub(r"\s+", "-", re.sub(r"[^\w\s-]", "", plain).strip())


def inline(text: str, resolve: LinkResolver) -> str:
    """Render inline Markdown, resolving link targets through the callback."""
    pieces: list[str] = []
    position = 0
    for match in INLINE.finditer(text):
        pieces.append(html.escape(text[position : match.start()]))
        position = match.end()
        if match.group("code") is not None:
            pieces.append(f"<code>{html.escape(match.group('code'))}</code>")
        elif match.group("bold") is not None:
            pieces.append(f"<strong>{inline(match.group('bold'), resolve)}</strong>")
        elif match.group("italic") is not None:
            pieces.append(f"<em>{inline(match.group('italic'), resolve)}</em>")
        else:
            target = html.escape(resolve(match.group("target")), quote=True)
            label = inline(match.group("label"), resolve)
            pieces.append(f'<a href="{target}">{label}</a>')
    pieces.append(html.escape(text[position:]))
    return "".join(pieces)


def split_row(line: str) -> list[str]:
    """Split a pipe-table row, ignoring pipes inside inline code."""
    cells: list[str] = []
    current: list[str] = []
    in_code = False
    for char in line.strip().removeprefix("|"):
        if char == "`":
            in_code = not in_code
        if char == "|" and not in_code:
            cells.append("".join(current).strip())
            current = []
        else:
            current.append(char)
    if "".join(current).strip():
        cells.append("".join(current).strip())
    return cells


def is_table_start(lines: list[str], index: int) -> bool:
    """Return whether a header row is followed by a pipe-table separator row."""
    if not lines[index].lstrip().startswith("|") or index + 1 >= len(lines):
        return False
    separator = split_row(lines[index + 1])
    return bool(separator) and all(SEPARATOR_CELL.fullmatch(c) for c in separator)


def render_table(rows: list[str], resolve: LinkResolver) -> str:
    """Render a header, separator and body rows as an accessible table."""
    head = split_row(rows[0])
    align = [
        (
            "right"
            if cell.endswith(":") and not cell.startswith(":")
            else "center" if cell.startswith(":") and cell.endswith(":") else ""
        )
        for cell in split_row(rows[1])
    ]

    def cell(tag: str, value: str, column: int) -> str:
        css = ' class="num"' if column < len(align) and align[column] == "right" else ""
        scope = ' scope="col"' if tag == "th" else ""
        return f"<{tag}{css}{scope}>{inline(value, resolve)}</{tag}>"

    out = ['<div class="table-wrap"><table><thead><tr>']
    out.extend(cell("th", value, i) for i, value in enumerate(head))
    out.append("</tr></thead><tbody>")
    for line in rows[2:]:
        values = split_row(line)
        out.append("<tr>")
        out.extend(cell("td", value, i) for i, value in enumerate(values))
        out.append("</tr>")
    out.append("</tbody></table></div>")
    return "".join(out)


def starts_block(lines: list[str], index: int) -> bool:
    """Return whether the line at index begins a non-paragraph block."""
    line = lines[index].strip()
    return (
        line.startswith(("```", ">"))
        or any(p.fullmatch(line) for p in (HEADING, BULLET, NUMBERED))
        or is_table_start(lines, index)
    )


def render_list(items: list[str], ordered: bool, resolve: LinkResolver) -> str:
    """Render collected list items as an ordered or unordered list."""
    tag = "ol" if ordered else "ul"
    body = "".join(f"<li>{inline(item, resolve)}</li>" for item in items)
    return f"<{tag}>{body}</{tag}>"


def render(text: str, resolve: LinkResolver, heading_offset: int = 0) -> str:
    """Render a Markdown fragment to HTML.

    heading_offset shifts heading levels so a topic's own title and rule headings
    nest under the page's structure.
    """
    # A single state machine keeps block boundaries visible; splitting hides them.
    # pylint: disable=too-many-branches,too-many-locals,too-many-statements
    lines = text.split("\n")
    out: list[str] = []
    index = 0
    while index < len(lines):
        line = lines[index]
        stripped = line.strip()
        if not stripped:
            index += 1
        elif stripped.startswith("```"):
            language = stripped.removeprefix("```").strip()
            code: list[str] = []
            index += 1
            while index < len(lines) and not lines[index].strip().startswith("```"):
                code.append(lines[index])
                index += 1
            index += 1
            css = f' class="language-{html.escape(language)}"' if language else ""
            body = html.escape("\n".join(code))
            out.append(f"<pre><code{css}>{body}</code></pre>")
        elif match := HEADING.fullmatch(stripped):
            level = min(6, len(match.group(1)) + heading_offset)
            title = match.group(2)
            out.append(
                f'<h{level} id="{slugify(title)}">{inline(title, resolve)}</h{level}>'
            )
            index += 1
        elif is_table_start(lines, index):
            rows = [lines[index], lines[index + 1]]
            index += 2
            while index < len(lines) and lines[index].lstrip().startswith("|"):
                rows.append(lines[index])
                index += 1
            out.append(render_table(rows, resolve))
        elif stripped.startswith(">"):
            quote: list[str] = []
            while index < len(lines) and lines[index].strip().startswith(">"):
                quote.append(lines[index].strip().removeprefix(">").strip())
                index += 1
            out.append(
                f"<blockquote>{render(chr(10).join(quote), resolve)}</blockquote>"
            )
        elif BULLET.fullmatch(stripped) or NUMBERED.fullmatch(stripped):
            ordered = NUMBERED.fullmatch(stripped) is not None
            marker = NUMBERED if ordered else BULLET
            items: list[str] = []
            while index < len(lines) and lines[index].strip():
                item = marker.fullmatch(lines[index].strip())
                if item:
                    items.append(item.group(1))
                elif not lines[index].strip().startswith(("```", "|", ">", "#")):
                    items[-1] += " " + lines[index].strip()
                else:
                    break
                index += 1
            out.append(render_list(items, ordered, resolve))
        else:
            paragraph: list[str] = []
            while index < len(lines) and lines[index].strip():
                follower = lines[index].strip()
                if paragraph and starts_block(lines, index):
                    break
                paragraph.append(follower)
                index += 1
            out.append(f"<p>{inline(' '.join(paragraph), resolve)}</p>")
    return "\n".join(out)
