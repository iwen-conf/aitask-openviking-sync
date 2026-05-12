import type { MemoryItem } from '@/api/types'

/** 把扁平 items 列表按 uri 路径段聚合成树。 */
export interface MemoryNode {
  name: string
  path: string
  uri?: string
  type?: MemoryItem['type']
  children: MemoryNode[]
}

function uriToSegments(rootUri: string, item: MemoryItem): string[] {
  const sub = item.uri.startsWith(rootUri)
    ? item.uri.slice(rootUri.length).replace(/^\/+/, '')
    : item.uri
  return sub.split('/').filter(Boolean)
}

export function buildMemoryTree(rootUri: string, items: MemoryItem[]): MemoryNode {
  const root: MemoryNode = { name: '/', path: '', children: [] }
  for (const item of items) {
    const segments = uriToSegments(rootUri, item)
    if (segments.length === 0) continue
    let cursor = root
    segments.forEach((segment, index) => {
      const isLeaf = index === segments.length - 1
      let next = cursor.children.find((child) => child.name === segment)
      if (!next) {
        next = {
          name: segment,
          path: segments.slice(0, index + 1).join('/'),
          children: [],
          ...(isLeaf ? { uri: item.uri, type: item.type } : {}),
        }
        cursor.children.push(next)
      } else if (isLeaf) {
        next.uri = item.uri
        next.type = item.type
      }
      cursor = next
    })
  }
  sortTree(root)
  return root
}

function sortTree(node: MemoryNode): void {
  node.children.sort((a, b) => {
    const aIsDir = a.children.length > 0
    const bIsDir = b.children.length > 0
    if (aIsDir !== bIsDir) return aIsDir ? -1 : 1
    return a.name.localeCompare(b.name)
  })
  node.children.forEach(sortTree)
}

export function findFirstLeaf(node: MemoryNode): MemoryNode | undefined {
  if (node.uri) return node
  for (const child of node.children) {
    const leaf = findFirstLeaf(child)
    if (leaf) return leaf
  }
  return undefined
}
