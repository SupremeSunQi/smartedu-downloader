import {BookOpen} from 'lucide-react';
import type {Textbook} from '../shared/types';
import {TextbookRow} from './TextbookRow';

interface TextbookListProps {
  books: Textbook[];
  loading: boolean;
  busyBookId: string;
  hasFilters: boolean;
  onDownload(id: string): void;
}

export function TextbookList({
  books, loading, busyBookId, hasFilters, onDownload,
}: TextbookListProps) {
  if (loading && books.length === 0) {
    return (
      <div className="textbook-list textbook-list--skeleton" role="status" aria-live="polite" aria-busy="true" aria-label="正在加载教材">
        {[0, 1, 2, 3].map((index) => (
          <div className="skeleton-row" key={index} aria-hidden="true">
            <div className="skeleton-cover" />
            <div className="skeleton-copy">
              <span className="skeleton-title" />
              <span className="skeleton-tags"><i /><i /><i /></span>
              <span className="skeleton-meta" />
            </div>
            <span className="skeleton-action" />
          </div>
        ))}
      </div>
    );
  }
  if (books.length === 0) {
    return (
      <div className="catalog-empty">
        <BookOpen size={25} aria-hidden="true" />
        <strong>{hasFilters ? '没有符合条件的教材' : '目录中暂时没有教材'}</strong>
        <span>{hasFilters ? '请选择其他教材条件' : '请刷新目录后重试'}</span>
      </div>
    );
  }
  return (
    <div className={`textbook-list${loading ? ' is-loading' : ''}`} aria-busy={loading}>
      {books.map((book) => (
        <TextbookRow
          key={book.id}
          book={book}
          busy={busyBookId === book.id}
          onDownload={() => onDownload(book.id)}
        />
      ))}
    </div>
  );
}
