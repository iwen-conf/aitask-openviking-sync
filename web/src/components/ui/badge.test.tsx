import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { Badge } from './badge'

describe('Badge', () => {
  it('renders children inside a span by default', () => {
    render(<Badge>committed</Badge>)
    const node = screen.getByText('committed')
    expect(node.tagName.toLowerCase()).toBe('span')
  })

  it('applies tone-specific classes', () => {
    render(<Badge tone="success">ok</Badge>)
    expect(screen.getByText('ok').className).toMatch(/emerald/)
  })

  it('forwards extra className without losing variant classes', () => {
    render(
      <Badge tone="warning" className="data-test-extra">
        warn
      </Badge>,
    )
    const node = screen.getByText('warn')
    expect(node.className).toMatch(/amber/)
    expect(node.className).toMatch(/data-test-extra/)
  })
})
