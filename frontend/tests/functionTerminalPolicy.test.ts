import { describe, expect, it } from 'vitest'
import {
  functionEntryTypeId,
  functionReturnTypeId,
  isCopyableFunctionNode,
  isPasteableFunctionNode,
  planFunctionTerminalDeletion,
  type FunctionTerminalNodeIdentity
} from '../src/editor/functionTerminalPolicy'

function node(id: string, typeId: string): FunctionTerminalNodeIdentity {
  return { id, typeId }
}

describe('function terminal deletion policy', () => {
  it('protects the only entry and the only return node', () => {
    const entry = node('entry', functionEntryTypeId)
    const returnNode = node('return', functionReturnTypeId)

    expect(planFunctionTerminalDeletion([entry, returnNode], [entry, returnNode])).toEqual({
      deletableIds: [],
      protectedEntryIds: ['entry'],
      protectedReturnIds: ['return']
    })
  })

  it('allows duplicate entries to be removed while preserving one for legacy repair', () => {
    const first = node('entry-1', functionEntryTypeId)
    const duplicate = node('entry-2', functionEntryTypeId)

    expect(planFunctionTerminalDeletion([first, duplicate], [duplicate]).deletableIds).toEqual(['entry-2'])
    expect(planFunctionTerminalDeletion([first, duplicate], [first, duplicate])).toEqual({
      deletableIds: ['entry-2'],
      protectedEntryIds: ['entry-1'],
      protectedReturnIds: []
    })
  })

  it('allows multiple selected returns to be removed as long as one remains', () => {
    const first = node('return-1', functionReturnTypeId)
    const second = node('return-2', functionReturnTypeId)
    const third = node('return-3', functionReturnTypeId)

    expect(planFunctionTerminalDeletion([first, second, third], [second, third]).deletableIds).toEqual(['return-2', 'return-3'])
    expect(planFunctionTerminalDeletion([first, second, third], [first, second, third])).toEqual({
      deletableIds: ['return-2', 'return-3'],
      protectedEntryIds: [],
      protectedReturnIds: ['return-1']
    })
  })

  it('keeps ordinary selected nodes deletable when terminals are protected', () => {
    const entry = node('entry', functionEntryTypeId)
    const returnNode = node('return', functionReturnTypeId)
    const work = node('work', 'origin.test.log')

    expect(planFunctionTerminalDeletion([entry, returnNode, work], [entry, returnNode, work])).toEqual({
      deletableIds: ['work'],
      protectedEntryIds: ['entry'],
      protectedReturnIds: ['return']
    })
  })
})

describe('function entry clipboard policy', () => {
  const entry = node('entry', functionEntryTypeId)
  const returnNode = node('return', functionReturnTypeId)

  it('prevents entry duplication through copy and paste', () => {
    expect(isCopyableFunctionNode(entry)).toBe(false)
    expect(isPasteableFunctionNode(entry)).toBe(false)
  })

  it('allows return nodes to be copied for early-return branches', () => {
    expect(isCopyableFunctionNode(returnNode)).toBe(true)
    expect(isPasteableFunctionNode(returnNode)).toBe(true)
  })
})
