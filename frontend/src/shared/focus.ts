import {useEffect, useRef} from 'react';

const focusableSelector = 'a[href], button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [tabindex]:not([tabindex="-1"])';

export function trapTabKey(event: KeyboardEvent, container: HTMLElement) {
  if (event.key !== 'Tab') return;
  const items = Array.from(container.querySelectorAll<HTMLElement>(focusableSelector))
    .filter((item) => !item.closest('[hidden], [inert], [aria-hidden="true"]'));
  const first = items[0];
  const last = items[items.length - 1];
  if (!first || !last) {
    event.preventDefault();
    container.focus();
  } else if (!container.contains(document.activeElement) || document.activeElement === container) {
    event.preventDefault();
    (event.shiftKey ? last : first).focus();
  } else if (event.shiftKey && document.activeElement === first) {
    event.preventDefault();
    last.focus();
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault();
    first.focus();
  }
}

export function useModalFocus<T extends HTMLElement>(active = true) {
  const ref = useRef<T>(null);
  useEffect(() => {
    if (!active || !ref.current) return;
    const dialog = ref.current;
    const scrim = dialog.closest<HTMLElement>('.dialog-scrim');
    const previousFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const siblings = scrim?.parentElement
      ? Array.from(scrim.parentElement.children).filter((item): item is HTMLElement => item instanceof HTMLElement && item !== scrim)
      : [];
    const priorAttributes = siblings.map((item) => ({
      item, inert: item.hasAttribute('inert'), ariaHidden: item.getAttribute('aria-hidden'),
    }));
    siblings.forEach((item) => {
      item.setAttribute('inert', '');
      item.setAttribute('aria-hidden', 'true');
    });
    dialog.focus();
    const onKeyDown = (event: KeyboardEvent) => {
      const openScrims = document.querySelectorAll('.dialog-scrim');
      if (openScrims[openScrims.length - 1] === scrim) trapTabKey(event, dialog);
    };
    document.addEventListener('keydown', onKeyDown);
    return () => {
      document.removeEventListener('keydown', onKeyDown);
      priorAttributes.forEach(({item, inert, ariaHidden}) => {
        if (!inert) item.removeAttribute('inert');
        if (ariaHidden === null) item.removeAttribute('aria-hidden');
        else item.setAttribute('aria-hidden', ariaHidden);
      });
      if (previousFocus?.isConnected) previousFocus.focus();
    };
  }, [active]);
  return ref;
}
