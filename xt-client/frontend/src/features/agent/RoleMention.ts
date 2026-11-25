import {Node} from '@tiptap/core';
import type {Editor} from '@tiptap/core';
import type {OfficeMember} from '../../lib/office';

export const roleMentionType = 'officeRoleMention';

export const RoleMention = Node.create({
  name: roleMentionType,
  group: 'inline',
  inline: true,
  atom: true,
  addAttributes() {
    return {
      memberId: {default: '', parseHTML: element => element.getAttribute('data-member-id') || ''},
      name: {default: '', parseHTML: element => element.textContent?.replace(/^@/, '') || ''},
    };
  },
  parseHTML() {
    return [{tag: 'span[data-office-role-mention]'}];
  },
  renderHTML({node}) {
    return ['span', {'data-office-role-mention': '', 'data-member-id': node.attrs.memberId, class: 'agent-conversation-reference agent-role-mention', title: `办公室员工：${node.attrs.name}`}, `@${node.attrs.name}`];
  },
  renderText({node}) {
    return `@${node.attrs.name}`;
  },
});

export function insertRoleMention(editor: Editor, from: number, member: OfficeMember) {
  const node = editor.schema.nodes[roleMentionType].create({memberId: member.id, name: member.name});
  const change = editor.state.tr.replaceWith(from, editor.state.selection.from, node);
  editor.view.dispatch(change.insertText(' ', from + node.nodeSize).scrollIntoView());
  editor.commands.focus();
}

export function editorRoleMentions(editor: Editor) {
  const members = new Map<string, string>();
  editor.state.doc.descendants(node => {
    if (node.type.name === roleMentionType && node.attrs.memberId) members.set(node.attrs.memberId, node.attrs.name);
  });
  return members;
}
