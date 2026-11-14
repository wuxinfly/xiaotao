import {EditorContent, useEditor} from '@tiptap/react';
import type {Editor, EditorOptions, JSONContent} from '@tiptap/core';
import {TextSelection, type EditorState} from '@tiptap/pm/state';
import type {EditorView} from '@tiptap/pm/view';
import {DOMParser as ProseMirrorDOMParser, Fragment, Slice} from '@tiptap/pm/model';
import StarterKit from '@tiptap/starter-kit';
import Placeholder from '@tiptap/extension-placeholder';
import {renderToStaticMarkup} from 'react-dom/server';
import Markdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import remarkBreaks from 'remark-breaks';
import {conversationReferenceType} from '../ConversationReference';

type Options = {
  label: string;
  placeholder?: string;
  extensions?: EditorOptions['extensions'];
  handlePaste?: EditorOptions['editorProps']['handlePaste'];
  handleKeyDown?: EditorOptions['editorProps']['handleKeyDown'];
  onUpdate: (editor: Editor) => void;
  onSend: () => boolean;
};

export function ChatEditor({editor}: {editor: Editor | null}) {
  return <EditorContent editor={editor} className="agent-editor"/>;
}

export function useChatEditor({label, placeholder = '描述你想完成的事…', extensions = [], handlePaste, handleKeyDown, onUpdate, onSend}: Options) {
  return useEditor({
    extensions: [StarterKit.configure({trailingNode: false}), Placeholder.configure({placeholder}), ...extensions],
    editorProps: {
      attributes: {'aria-label': label, role: 'textbox', 'aria-multiline': 'true'},
      handlePaste(view, event, slice) {
        if (handlePaste?.(view, event, slice)) return true;
        if (hasActiveNode(view.state, ['codeBlock'])) return false;
        const text = event.clipboardData?.getData('text/plain').replace(/\r\n?/g, '\n');
        const match = text?.match(/^\s*(`{3,}|~{3,})(?:markdown|md)[ \t]*\n([\s\S]*?)\n\1[ \t]*\s*$/i);
        if (text && !match && !text.includes('\n') && /\\/.test(text) && !/^[a-z][a-z\d+.-]*:\/\//i.test(text)) {
          view.dispatch(view.state.tr.insertText(text).setMeta('preventAutolink', true).scrollIntoView());
          event.preventDefault();
          return true;
        }
        const html = event.clipboardData?.getData('text/html') ?? '';
        const source = new DOMParser().parseFromString(html, 'text/html');
        const codeContainer = source.body.querySelector('pre') || Array.from(source.body.querySelectorAll<HTMLElement>('[style]')).find(element => element.style.whiteSpace === 'pre');
        if (!match && text && codeContainer) {
          const nodes = text.split('\n').flatMap((line, index) => [
            ...(index ? [view.state.schema.nodes.hardBreak.create()] : []),
            ...(line ? [view.state.schema.text(line)] : []),
          ]);
          view.dispatch(view.state.tr.replaceSelection(new Slice(Fragment.fromArray(nodes), 0, 0)).scrollIntoView());
          event.preventDefault();
          return true;
        }
        if (!match) return false;
        const container = document.createElement('div');
        container.innerHTML = renderToStaticMarkup(<Markdown remarkPlugins={[remarkGfm, remarkBreaks]}>{match[2]}</Markdown>);
        const content = ProseMirrorDOMParser.fromSchema(view.state.schema).parseSlice(container);
        view.dispatch(view.state.tr.replaceSelection(content).scrollIntoView());
        event.preventDefault();
        return true;
      },
      handleTextInput: (view, from, to, text) => {
        if (text !== ' ' || from !== to || hasActiveNode(view.state, ['codeBlock'])) return false;
        const { $from } = view.state.selection;
        const before = $from.parent.textBetween(0, $from.parentOffset, '\n', '\ufffc');
        const match = /(^|[^`])`([^`\n]+)`$/.exec(before);
        if (!match || !view.state.schema.marks.code) return false;
        const start = from - match[2].length - 2;
        let formatted = false;
        view.state.doc.nodesBetween(start, from, node => { if (node.isText && node.marks.length) formatted = true; });
        if (formatted) return false;
        const code = view.state.schema.text(match[2], [view.state.schema.marks.code.create()]);
        const space = view.state.schema.text(' ');
        view.dispatch(view.state.tr.replaceWith(start, to, [code, space]).scrollIntoView());
        return true;
      },
      handleKeyDown(view, event) {
        if (handleKeyDown?.(view, event)) return true;
        if (event.key !== 'Enter' || event.isComposing) return false;
        if (event.ctrlKey || event.metaKey) return submit(event, onSend);
        if (hasActiveNode(view.state, ['codeBlock'])) return false;
        if (hasActiveNode(view.state, ['listItem'])) {
          if (!isEmptyListItem(view.state) || !exitEmptyListItem(view)) return false;
          event.preventDefault();
          return true;
        }
        if (event.shiftKey || hasActiveNode(view.state, ['heading', 'blockquote'])) return false;
        return submit(event, onSend);
      },
    },
    onUpdate: ({editor}) => onUpdate(editor),
  });
}

function submit(event: KeyboardEvent, onSend: () => boolean): boolean {
  if (!onSend()) return false;
  event.preventDefault();
  return true;
}

function inlineMarkdown(node: JSONContent): string {
  if (node.type === conversationReferenceType) return '@' + String(node.attrs?.title || '');
  if (node.type === 'text') {
    let text = node.text ?? '';
    for (const mark of node.marks ?? []) {
      if (mark.type === 'code') text = '`' + text + '`';
      else if (mark.type === 'bold') text = '**' + text + '**';
      else if (mark.type === 'italic') text = '*' + text + '*';
      else if (mark.type === 'strike') text = '~~' + text + '~~';
      else if (mark.type === 'underline') text = '<u>' + text + '</u>';
      else if (mark.type === 'link') text = '[' + text + '](' + (mark.attrs?.href ?? '') + ')';
    }
    return text;
  }
  if (node.type === 'hardBreak') return '  \n';
  const content = (node.content ?? []).map(inlineMarkdown).join('');
  return content;
}

function blocksMarkdown(nodes: JSONContent[], indent = 0): string {
  return nodes.map(node => blockMarkdown(node, indent)).filter(Boolean).join('\n\n');
}

function listMarkdown(node: JSONContent, indent: number): string {
  const ordered = node.type === 'orderedList';
  const start = Number(node.attrs?.start) || 1;
  return (node.content ?? []).map((item, index) => {
    const children = item.content ?? [];
    const first = children[0];
    const prefix = ' '.repeat(indent) + (ordered ? (start + index) + '. ' : '- ');
    const head = first ? inlineMarkdown(first).replace(/\n/g, '\n' + ' '.repeat(prefix.length)) : '';
    const rest = children.slice(1);
    const nested = rest.length ? '\n' + blocksMarkdown(rest, indent + 2).replace(/\n\n/g, '\n') : '';
    return prefix + head + nested;
  }).join('\n');
}

function blockMarkdown(node: JSONContent, indent: number): string {
  if (node.type === 'paragraph') return ' '.repeat(indent) + inlineMarkdown(node);
  if (node.type === 'heading') {
    const level = Math.min(6, Math.max(1, Number(node.attrs?.level) || 1));
    return ' '.repeat(indent) + '#'.repeat(level) + ' ' + inlineMarkdown(node);
  }
  if (node.type === 'bulletList' || node.type === 'orderedList') return listMarkdown(node, indent);
  if (node.type === 'blockquote') {
    return (node.content ?? []).map(child => blockMarkdown(child, 0).split('\n').map(line => ' '.repeat(indent) + '> ' + line).join('\n')).join('\n\n');
  }
  if (node.type === 'codeBlock') {
    const language = String(node.attrs?.language || '');
    return ' '.repeat(indent) + '```' + language + '\n' + (node.content ?? []).map(child => child.text ?? '').join('') + '\n' + ' '.repeat(indent) + '```';
  }
  if (node.type === 'horizontalRule') return ' '.repeat(indent) + '---';
  return ' '.repeat(indent) + inlineMarkdown(node);
}

export function editorMarkdown(editor: Editor): string {
  return blocksMarkdown(editor.getJSON().content ?? []).trim();
}

export function hasActiveNode(state: EditorState, names: string[]): boolean {
  const { $from } = state.selection;
  for (let depth = $from.depth; depth > 0; depth -= 1) {
    if (names.includes($from.node(depth).type.name)) return true;
  }
  return false;
}

function isEmptyListItem(state: EditorState): boolean {
  const { $from, $to } = state.selection;
  return $from.parentOffset === 0 && $to.parentOffset === 0 &&
    $from.parent.content.size === 0 && hasActiveNode(state, ['listItem']);
}

function exitEmptyListItem(view: EditorView): boolean {
  const { state } = view;
  const { $from } = state.selection;
  let listDepth = -1;
  for (let depth = $from.depth; depth > 0; depth -= 1) {
    if ($from.node(depth).type.name === 'listItem') {
      listDepth = depth;
      break;
    }
  }
  if (listDepth < 0) return false;

  const list = $from.node(listDepth - 1);
  const listStart = $from.before(listDepth - 1);
  const itemIndex = $from.index(listDepth - 1);
  const item = list.child(itemIndex);
  if (itemIndex !== list.childCount - 1 || !item.firstChild?.isTextblock || item.firstChild.content.size) return false;

  const paragraph = state.schema.nodes.paragraph.create();
  const transaction = state.tr;
  if (list.childCount === 1) {
    transaction.replaceWith(listStart, listStart + list.nodeSize, paragraph);
  } else {
    transaction.delete($from.before(listDepth), $from.after(listDepth));
    const afterList = listStart + list.nodeSize - item.nodeSize - 1;
    transaction.insert(afterList, paragraph);
  }
  const after = listStart + list.nodeSize - item.nodeSize + 1;
  transaction.setSelection(TextSelection.near(transaction.doc.resolve(Math.min(after, transaction.doc.content.size))));
  view.dispatch(transaction.scrollIntoView());
  return true;
}
