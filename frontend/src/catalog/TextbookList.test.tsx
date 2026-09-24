import {render, screen} from '@testing-library/react';
import {expect, it, vi} from 'vitest';
import {TextbookList} from './TextbookList';

it('renders textbook-shaped loading skeleton rows', () => {
  render(<TextbookList books={[]} loading busyBookId="" hasFilters={false} onDownload={vi.fn()} />);

  const list = screen.getByRole('status', {name: '正在加载教材'});
  const rows = list.querySelectorAll('.skeleton-row');

  expect(rows).toHaveLength(4);
  expect(rows[0].querySelector('.skeleton-cover')).not.toBeNull();
  expect(rows[0].querySelector('.skeleton-copy')).not.toBeNull();
  expect(rows[0].querySelector('.skeleton-title')).not.toBeNull();
  expect(rows[0].querySelector('.skeleton-meta')).not.toBeNull();
  expect(rows[0].querySelector('.skeleton-action')).not.toBeNull();
});

it('uses a consistent icon and concise copy for an empty result', () => {
  render(<TextbookList books={[]} loading={false} busyBookId="" hasFilters onDownload={vi.fn()} />);

  expect(screen.getByText('没有符合条件的教材')).toBeVisible();
  expect(screen.getByText('没有符合条件的教材').closest('.catalog-empty')?.querySelector('.lucide-book-open')).not.toBeNull();
});
