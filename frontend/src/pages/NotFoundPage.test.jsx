import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it } from 'vitest'
import { NotFoundPage } from './NotFoundPage'

describe('NotFoundPage', () => {
  it('offers a safe route back to the dashboard', () => {
    render(<MemoryRouter><NotFoundPage /></MemoryRouter>)
    expect(screen.getByRole('heading', { name: /panel does not exist/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /return to dashboard/i })).toHaveAttribute('href', '/')
  })
})
