/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import DOMPurify from 'dompurify'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { useAuthStore } from '@/stores/auth-store'
import { Markdown } from '@/components/ui/markdown'
import { PublicLayout } from '@/components/layout'
import { Footer } from '@/components/layout/components/footer'
import { CTA, Features, Hero, HowItWorks, Stats } from './components'
import { useHomePageContent } from './hooks'

const styleTagPattern = /<style\b[^>]*>([\s\S]*?)<\/style>/gi

const customHomeSanitizeOptions = {
  ADD_ATTR: ['class', 'style', 'target'],
} as const

function hasEmbeddedStyles(content: string): boolean {
  return /<style\b/i.test(content)
}

function addExternalLinkAttributes(html: string): string {
  if (typeof window === 'undefined') {
    return html
  }

  const template = document.createElement('template')
  template.innerHTML = html

  template.content.querySelectorAll('a[href]').forEach((link) => {
    link.setAttribute('target', '_blank')
    link.setAttribute('rel', 'noopener noreferrer')
  })

  return template.innerHTML
}

function CustomHomeContent({ content }: { content: string }) {
  const renderedContent = useMemo(() => {
    const styles: string[] = []
    const htmlWithoutStyles = content.replace(
      styleTagPattern,
      (_match, css: string) => {
        styles.push(css)
        return ''
      }
    )
    const sanitizedHtml = DOMPurify.sanitize(
      htmlWithoutStyles,
      customHomeSanitizeOptions
    )

    return {
      html: addExternalLinkAttributes(sanitizedHtml),
      styles: styles.join('\n'),
    }
  }, [content])

  return (
    <>
      {renderedContent.styles && (
        <style data-custom-home-content>{renderedContent.styles}</style>
      )}
      <div
        className='custom-home-content'
        dangerouslySetInnerHTML={{ __html: renderedContent.html }}
      />
    </>
  )
}

export function Home() {
  const { t } = useTranslation()
  const { auth } = useAuthStore()
  const isAuthenticated = !!auth.user
  const { content, isLoaded, isUrl } = useHomePageContent()

  if (!isLoaded) {
    return (
      <PublicLayout showMainContainer={false}>
        <main className='flex min-h-screen items-center justify-center'>
          <div className='text-muted-foreground'>{t('Loading...')}</div>
        </main>
      </PublicLayout>
    )
  }

  if (content) {
    return (
      <PublicLayout showMainContainer={false}>
        <main className='overflow-x-hidden'>
          {isUrl ? (
            <iframe
              src={content}
              className='h-screen w-full border-none'
              title={t('Custom Home Page')}
            />
          ) : (
            <div className='container mx-auto py-8'>
              {hasEmbeddedStyles(content) ? (
                <CustomHomeContent content={content} />
              ) : (
                <Markdown className='custom-home-content'>{content}</Markdown>
              )}
            </div>
          )}
        </main>
      </PublicLayout>
    )
  }

  return (
    <PublicLayout showMainContainer={false}>
      <Hero isAuthenticated={isAuthenticated} />
      <Stats />
      <Features />
      <HowItWorks />
      <CTA isAuthenticated={isAuthenticated} />
      <Footer />
    </PublicLayout>
  )
}
