(() => {
  'use strict';

  const INDENT = 4;

  // Splits a line into the brackets that count for indentation, skipping
  // strings, string interpolations and comments. A line can leave a block
  // comment or a multi-line string open for the lines after it.
  function scanLine(text, state) {
    const brackets = [];
    let codeEnd = 0;
    let i = 0;
    if (state.inString) {
      const end = text.indexOf('"""');
      if (end < 0) return { brackets, code: '' };
      state.inString = false;
      i = end + 3;
    }
    if (state.inComment) {
      const end = text.indexOf('*/');
      if (end < 0) return { brackets, code: '' };
      state.inComment = false;
      i = end + 2;
    }
    while (i < text.length) {
      const ch = text[i];
      if (ch === '/' && text[i + 1] === '/') break;
      if (ch === '/' && text[i + 1] === '*') {
        const end = text.indexOf('*/', i + 2);
        if (end < 0) { state.inComment = true; break; }
        i = end + 2;
        continue;
      }
      if (text.startsWith('"""', i)) {
        const end = text.indexOf('"""', i + 3);
        if (end < 0) { state.inString = true; break; }
        i = end + 3;
        codeEnd = i;
        continue;
      }
      if (ch === '"') {
        i = skipString(text, i + 1);
        codeEnd = i;
        continue;
      }
      if ('{([)]}'.includes(ch)) brackets.push({ ch, index: i });
      if (!/\s/.test(ch)) codeEnd = i + 1;
      i++;
    }
    return { brackets, code: text.slice(0, codeEnd) };
  }

  // Returns the index after the closing quote of a string that starts at
  // start, stepping over \(…) interpolations, which can hold strings too.
  function skipString(text, start) {
    let i = start;
    while (i < text.length) {
      if (text[i] === '\\' && text[i + 1] === '(') {
        let depth = 1;
        i += 2;
        while (i < text.length && depth > 0) {
          if (text[i] === '"') { i = skipString(text, i + 1); continue; }
          if (text[i] === '(') depth++;
          if (text[i] === ')') depth--;
          i++;
        }
        continue;
      }
      if (text[i] === '\\') { i += 2; continue; }
      if (text[i] === '"') return i + 1;
      i++;
    }
    return i;
  }

  function leadingWidth(line) {
    let width = 0;
    for (const ch of line) {
      if (ch === ' ') width++;
      else if (ch === '\t') width += INDENT;
      else break;
    }
    return width;
  }

  // Re-indents Swift source lines. Each bracket opened and left open on a line
  // indents the following lines one level; several opened on the same line,
  // like Button(action: {, still count as one level. On top of that:
  // - case and default line up with their switch,
  // - a line starting with .modifier continues the expression above it,
  // - a line continuing an expression (after =, &&, ||, …) is indented once,
  // - arguments lined up under an open "(" keep the spacing they have,
  // - multi-line strings and block comments are left as they are.
  // Trailing whitespace is removed, except on the caret's line.
  function reindent(lines, cursorLine) {
    const stack = [];
    const state = { inString: false, inComment: false };
    let previous = null;
    return lines.map((line, lineIndex) => {
      const keepEnd = lineIndex === cursorLine;
      if (state.inString || state.inComment) {
        const wasString = state.inString;
        // Code after the closing """ or */, such as the ")" of Text("""…"""), still counts.
        for (const bracket of scanLine(line, state).brackets) {
          const top = stack[stack.length - 1];
          if ('{(['.includes(bracket.ch)) stack.push({ ch: bracket.ch, width: (top ? top.width : 0) + INDENT, isSwitch: false, hanging: false, shift: 0, startsWithDot: false });
          else stack.pop();
        }
        if (wasString) return line;
        return keepEnd ? line : line.trimEnd();
      }
      const text = line.trim();
      if (!text) return keepEnd ? line : '';

      const { brackets, code } = scanLine(text, state);
      const isComment = text.startsWith('//') || text.startsWith('/*');
      const originalWidth = leadingWidth(line);

      // Closing brackets at the start of the line dedent the line itself.
      const closers = /^[\s)\]}]*/.exec(text)[0].length;
      let leading = 0;
      while (leading < brackets.length && brackets[leading].index < closers) leading++;
      let width;
      let bodyWidth = null;
      let closedOpener = null;
      for (let k = 0; k < leading; k++) closedOpener = stack.pop() || closedOpener;
      const top = stack[stack.length - 1];
      if (closedOpener) {
        width = closedOpener.width - INDENT;
      } else if (top && top.hanging) {
        width = originalWidth + top.shift;
      } else {
        width = top ? top.width : 0;
        if (top && top.isSwitch && /^(case\b|default\s*:|@unknown\s+default)/.test(text)) width -= INDENT;
        // "if let a = x,\n   let b = y {" keeps its alignment.
        // A brace at the end of such a line opens the body of the statement,
        // so the body is indented from where the statement starts.
        else if (!isComment && previous && previous.code.endsWith(',') && (!top || top.ch === '{')) {
          bodyWidth = previous.bodyWidth;
          width = Math.max(width, originalWidth + previous.shift);
        } else if (!isComment && previous && previous.chainWidth !== null && isModifierLine(text) && !/[,({[]$/.test(previous.code)) {
          width = previous.chainWidth;
        } else if (!isComment && previous && continues(text, previous, originalWidth)) {
          if (!isModifierLine(text)) bodyWidth = previous.bodyWidth;
          width += INDENT;
        }
      }
      width = Math.max(0, width);

      // Brackets after the leading closers open and close as usual; the ones
      // still open at the end of the line share one indentation level.
      const openedHere = [];
      for (let k = leading; k < brackets.length; k++) {
        const bracket = brackets[k];
        if ('{(['.includes(bracket.ch)) {
          const entry = { ch: bracket.ch, width: 0, isSwitch: false, hanging: false, shift: width - originalWidth, startsWithDot: text.startsWith('.') };
          if (bracket.ch === '{') entry.isSwitch = /\bswitch\b/.test(text.slice(0, bracket.index));
          else entry.hanging = text.slice(bracket.index + 1).replace(/\/\/.*$/, '').trim() !== '';
          stack.push(entry);
          openedHere.push(entry);
        } else {
          const popped = stack.pop();
          const at = openedHere.indexOf(popped);
          if (at >= 0) openedHere.splice(at, 1);
        }
      }
      if (bodyWidth === null) bodyWidth = width;
      for (const entry of openedHere) entry.width = bodyWidth + INDENT;

      if (!isComment) {
        // Later modifiers in a chain line up with the first one, however that
        // one was indented; the "}" closing a ".toolbar {" stays in the chain.
        const inChain = isModifierLine(text) || (closedOpener && closedOpener.startsWithDot && !openedHere.length);
        previous = { code, closedPlainOpener: closedOpener && !closedOpener.startsWithDot, originalWidth, shift: width - originalWidth, bodyWidth, chainWidth: inChain ? width : null };
      }
      return ' '.repeat(width) + text + (keepEnd ? line.slice(line.trimEnd().length) : '');
    });
  }

  function isModifierLine(text) {
    return /^\.[A-Za-z_]/.test(text);
  }

  // Whether a line carries on the expression of the line before it.
  function continues(text, previous, originalWidth) {
    if (isModifierLine(text)) {
      if (/([,({[]|\bin|\breturn)$/.test(previous.code)) return false;
      // "VStack { … }" followed by ".padding()" stays level when it was written level.
      if (previous.closedPlainOpener && originalWidth <= previous.originalWidth) return false;
      return true;
    }
    if (/^(&&|\|\||\?\?|\?|:|->|\+|-(?!\d)|\*|\/(?![/*])|where\b)/.test(text) && !/^(:|\?)\s*$/.test(text)) return true;
    return /(&&|\|\||\?\?|[=+*\-/]|->)$/.test(previous.code) && !/(==|!=|<=|>=)$/.test(previous.code);
  }

  window.SwiftFormat = { reindent };
})();
