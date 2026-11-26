// Regras compartilhadas pelos campos; não dependem de bibliotecas externas.
(() => {
  const compact = value => value.trim().toUpperCase().replace(/[.\-/]/g, '');
  const mod11 = sum => sum % 11 < 2 ? 0 : 11 - sum % 11;
  function validCPF(value) {
    if (!/^(?:\d{11}|\d{3}\.\d{3}\.\d{3}-\d{2})$/.test(value)) return false;
    const n = compact(value);
    if (/^(.)\1+$/.test(n)) return false;
    for (let size = 9; size <= 10; size++) {
      let sum = 0;
      for (let i = 0; i < size; i++) sum += Number(n[i]) * (size + 1 - i);
      if (Number(n[size]) !== mod11(sum)) return false;
    }
    return true;
  }
  function validCNPJ(value) {
    value = value.toUpperCase();
    if (!/^(?:[A-Z0-9]{12}\d{2}|[A-Z0-9]{2}\.[A-Z0-9]{3}\.[A-Z0-9]{3}\/[A-Z0-9]{4}-\d{2})$/.test(value)) return false;
    const n = compact(value);
    if (/^(.)\1+$/.test(n)) return false;
    for (let size = 12; size <= 13; size++) {
      let sum = 0, weight = 2;
      for (let i = size - 1; i >= 0; i--) {
        sum += (n.charCodeAt(i) - 48) * weight;
        if (++weight === 10) weight = 2;
      }
      if (Number(n[size]) !== mod11(sum)) return false;
    }
    return true;
  }
  function groupMask(raw, sizes, separators) {
    let out = '', offset = 0;
    for (let i = 0; i < sizes.length && offset < raw.length; i++) {
      if (i) out += separators[i - 1];
      out += raw.slice(offset, offset + sizes[i]);
      offset += sizes[i];
    }
    return out + raw.slice(offset);
  }
  function documentMask(value, cpfOnly = false) {
    const raw = value.toUpperCase().replace(/[.\-/\s]/g, '');
    if (!(cpfOnly ? /^\d*$/ : /^[A-Z0-9]*$/).test(raw)) return value;
    return !cpfOnly && (/[A-Z]/.test(raw) || raw.length > 11)
      ? groupMask(raw, [2, 3, 3, 4, 2], ['.', '.', '/', '-'])
      : groupMask(raw, [3, 3, 3, 2], ['.', '.', '-']);
  }
  function phoneMask(value) {
    const raw = value.replace(/[()\s-]/g, '');
    if (!/^\d*$/.test(raw) || !raw) return value;
    if (raw.length <= 2) return '(' + raw;
    const size = raw.length > 10 ? 5 : 4;
    return '(' + raw.slice(0, 2) + ') ' + groupMask(raw.slice(2), [size, 4], ['-']);
  }
  function validPhone(value) {
    if (!/^(?:\d{10,11}|\(\d{2}\) ?\d{4,5}-\d{4})$/.test(value)) return false;
    return /^[1-9]\d(?:[2-5]\d{7}|9\d{8})$/.test(value.replace(/[() -]/g, ''));
  }
  function moneyValue(value) {
    let raw = value.trim().replace(/^R\$\s*/, '');
    if (raw.includes(',')) {
      if (!/^(?:\d{1,9}|\d{1,3}(?:\.\d{3}){1,2}),\d{1,2}$/.test(raw)) return null;
      raw = raw.replace(/\./g, '').replace(',', '.');
    } else if (/^\d{1,3}(?:\.\d{3}){1,2}$/.test(raw)) raw = raw.replace(/\./g, '');
    if (!/^\d{1,9}(?:\.\d{1,2})?$/.test(raw)) return null;
    const [whole, fraction = ''] = raw.split('.');
    return Number(whole) + '.' + fraction.padEnd(2, '0');
  }
  function moneyMask(value) {
    const canonical = moneyValue(value);
    if (canonical === null) return value;
    const [whole, cents] = canonical.split('.');
    return Number(whole).toLocaleString('pt-BR') + ',' + cents;
  }
  function validEmail(value) {
    const local = value.split('@')[0];
    if (local.startsWith('.') || local.endsWith('.') || local.includes('..')) return false;
    return /^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]*[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]*[a-zA-Z0-9])?)+$/.test(value);
  }
  function validDate(value) {
    if (!/^(?!0000)\d{4}-\d{2}-\d{2}$/.test(value)) return false;
    const date = new Date(value + 'T00:00:00Z');
    return !isNaN(date) && date.toISOString().slice(0, 10) === value;
  }
  function message(kind, raw, required, max) {
    const value = raw.trim();
    if (!value) return required ? 'Preencha este campo.' : '';
    if ([...value].length > max) return `Use até ${max} caracteres.`;
    if (/[\u0000-\u001f\u007f-\u009f]/.test(value)) return 'Remova caracteres de controle e quebras de linha.';
    switch (kind) {
      case 'registration': return /^\d{1,17}$/.test(value) ? '' : 'Use somente números, até 17 dígitos.';
      case 'cpf': return validCPF(value) ? '' : 'CPF inválido. Confira os 11 dígitos.';
      case 'document': return validCPF(value) || validCNPJ(value) ? '' : 'CPF / CNPJ inválido. Confira o número e os dígitos verificadores.';
      case 'phone': return validPhone(value) ? '' : 'Informe DDD + telefone fixo ou celular (10 ou 11 dígitos).';
      case 'email': return validEmail(value) ? '' : 'Informe um e-mail válido, como nome@dominio.com.';
      case 'bankNumber': return /^\d+(?:-?[\dX])?$/i.test(value) ? '' : 'Use números e, se houver, dígito após um hífen. O dígito X é aceito.';
      case 'bank': if (/^\d+$/.test(value)) return ''; // Também aceita código bancário.
      case 'text': return [...value].length >= 2 && /\p{L}/u.test(value) ? '' : 'Informe um texto com pelo menos 2 caracteres.';
      case 'money': { const canonical = moneyValue(value); return canonical !== null && Number(canonical) > 0 ? '' : 'Informe de R$ 0,01 a R$ 999.999.999,99, com até 2 casas decimais.'; }
      case 'date': return validDate(value) ? '' : 'Informe uma data válida.';
      default: return '';
    }
  }
  const collator = new Intl.Collator('pt-BR', { sensitivity: 'base', numeric: true });
  function sortRecords(rows, key) {
    const name = row => key === 'conta' ? row.titular : key === 'prestador' ? row.descricao?.trim() || row.nome : row.nome;
    return [...rows].sort((a, b) => collator.compare(name(a), name(b))
      || (key === 'prestador' ? collator.compare(a.nome, b.nome) : 0)
      || collator.compare(a.banco || a.inscricao || a.documento || '', b.banco || b.inscricao || b.documento || '')
      || a.id - b.id);
  }
  globalThis.FormValidation = { validCPF, validCNPJ, documentMask, phoneMask, validPhone, moneyValue, moneyMask, validEmail, validDate, message, sortRecords };
})();
