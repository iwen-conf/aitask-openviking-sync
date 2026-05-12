import ReactMarkdown, { type Components } from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { cn } from '@/lib/utils'

const components: Components = {
  h1: ({ children, ...props }) => (
    <h1 className="mt-5 mb-2 text-2xl font-bold text-slate-900" {...props}>
      {children}
    </h1>
  ),
  h2: ({ children, ...props }) => (
    <h2 className="mt-5 mb-2 text-xl font-semibold text-slate-900" {...props}>
      {children}
    </h2>
  ),
  h3: ({ children, ...props }) => (
    <h3 className="mt-5 mb-2 text-lg font-semibold text-slate-900" {...props}>
      {children}
    </h3>
  ),
  h4: ({ children, ...props }) => (
    <h4 className="mt-5 mb-2 text-base font-semibold text-slate-900" {...props}>
      {children}
    </h4>
  ),
  h5: ({ children, ...props }) => (
    <h5
      className="mt-5 mb-2 text-sm font-semibold uppercase tracking-wide text-slate-900"
      {...props}
    >
      {children}
    </h5>
  ),
  h6: ({ children, ...props }) => (
    <h6
      className="mt-5 mb-2 text-xs font-semibold uppercase tracking-wide text-slate-900"
      {...props}
    >
      {children}
    </h6>
  ),
  p: ({ children, ...props }) => (
    <p className="my-2 leading-7 text-slate-700" {...props}>
      {children}
    </p>
  ),
  a: ({ children, href, ...props }) => (
    <a
      href={href}
      target="_blank"
      rel="noreferrer noopener"
      className="text-indigo-600 underline-offset-2 hover:underline"
      {...props}
    >
      {children}
    </a>
  ),
  strong: ({ children, ...props }) => (
    <strong className="font-semibold text-slate-900" {...props}>
      {children}
    </strong>
  ),
  em: ({ children, ...props }) => (
    <em className="italic text-slate-700" {...props}>
      {children}
    </em>
  ),
  del: ({ children, ...props }) => (
    <del className="text-slate-500 line-through" {...props}>
      {children}
    </del>
  ),
  ul: ({ children, ...props }) => (
    <ul className="my-2 list-disc space-y-1 pl-6" {...props}>
      {children}
    </ul>
  ),
  ol: ({ children, ...props }) => (
    <ol className="my-2 list-decimal space-y-1 pl-6" {...props}>
      {children}
    </ol>
  ),
  li: ({ children, ...props }) => (
    <li className="text-slate-700 marker:text-slate-400" {...props}>
      {children}
    </li>
  ),
  input: ({ type, checked, ...props }) =>
    type === 'checkbox' ? (
      <input
        type="checkbox"
        checked={checked}
        readOnly
        className="mr-2 h-3.5 w-3.5 translate-y-[1px] rounded border-slate-300 align-middle accent-indigo-500"
        {...props}
      />
    ) : (
      <input type={type} {...props} />
    ),
  blockquote: ({ children, ...props }) => (
    <blockquote
      className="my-3 border-l-4 border-indigo-200 bg-indigo-50/40 px-4 py-2 text-sm text-slate-600"
      {...props}
    >
      {children}
    </blockquote>
  ),
  hr: (props) => <hr className="my-4 border-slate-200" {...props} />,
  code: ({ className, children, ...props }) => {
    const isInline = !/language-/.test(className ?? '')
    if (isInline) {
      return (
        <code
          className="rounded bg-slate-100 px-1 py-0.5 font-mono text-[0.85em] text-slate-700"
          {...props}
        >
          {children}
        </code>
      )
    }
    return (
      <code className={cn('font-mono', className)} {...props}>
        {children}
      </code>
    )
  },
  pre: ({ children, ...props }) => (
    <pre
      className="my-3 overflow-x-auto rounded-lg bg-slate-950 p-4 text-xs leading-relaxed text-slate-100"
      {...props}
    >
      {children}
    </pre>
  ),
  table: ({ children, ...props }) => (
    <div className="my-3 overflow-x-auto">
      <table className="w-full text-sm" {...props}>
        {children}
      </table>
    </div>
  ),
  thead: ({ children, ...props }) => (
    <thead className="bg-slate-50 text-slate-600" {...props}>
      {children}
    </thead>
  ),
  th: ({ children, ...props }) => (
    <th className="border-b border-slate-200 px-3 py-2 text-left" {...props}>
      {children}
    </th>
  ),
  td: ({ children, ...props }) => (
    <td className="border-t border-slate-100 px-3 py-2 align-top" {...props}>
      {children}
    </td>
  ),
}

interface MarkdownProps {
  children: string
  className?: string
}

export function Markdown({ children, className }: MarkdownProps) {
  return (
    <div className={cn('text-sm', className)}>
      <ReactMarkdown remarkPlugins={[remarkGfm]} components={components}>
        {children}
      </ReactMarkdown>
    </div>
  )
}
