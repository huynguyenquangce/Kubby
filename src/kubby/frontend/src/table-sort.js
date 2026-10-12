// Values are interpreted as numbers only when the entire cell has that shape.
// Names such as 123-api stay names. Age minutes differ from CPU millicores.
export function tableSortValue(text, column = '') {
    const value = text.trim();
    if (/age/i.test(column)) {
        const units = { s: 1, m: 60, h: 3600, d: 86400, w: 604800, y: 31536000 };
        if (/^(?:[0-9]+(?:\.[0-9]+)?[smhdwy]\s*)+$/.test(value)) {
            return [...value.matchAll(/([0-9]+(?:\.[0-9]+)?)([smhdwy])/g)]
                .reduce((sum, [, number, unit]) => sum + Number(number) * units[unit], 0);
        }
    } else {
        const quantity = value.match(/^(-?[0-9]+(?:\.[0-9]+)?)\s*(Ki|Mi|Gi|Ti|Pi|K|M|G|T|m)?$/);
        if (quantity) {
            const factors = { Ki: 1024, Mi: 1024 ** 2, Gi: 1024 ** 3, Ti: 1024 ** 4, Pi: 1024 ** 5,
                K: 1000, M: 1e6, G: 1e9, T: 1e12, m: 0.001 };
            return Number(quantity[1]) * (factors[quantity[2]] ?? 1);
        }
        const fraction = value.match(/^([0-9]+)\/([0-9]+)(?:\s+ready)?$/);
        if (fraction) return Number(fraction[2]) ? Number(fraction[1]) / Number(fraction[2]) : -1;
    }
    return value.toLowerCase();
}
