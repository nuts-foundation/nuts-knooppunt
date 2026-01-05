/**
 * Mock organization data for e-Herkenning simulation
 */

export interface MockOrganization {
  id: string;
  name: string;
  type: 'pharmacy' | 'general_practice' | 'hospital' | 'care_home';
  typeLabel: string;
  typeLabelEn: string;
  agbCode: string;
  uraNumber: string;
}

export const mockOrganizations: MockOrganization[] = [
  {
    id: 'org-1',
    name: 'Apotheek De Zonnehoek',
    type: 'pharmacy',
    typeLabel: 'Apotheek',
    typeLabelEn: 'Pharmacy',
    agbCode: '06010713',
    uraNumber: '32475534',
  },
  {
    id: 'org-2',
    name: 'Huisartsenpraktijk Centrum',
    type: 'general_practice',
    typeLabel: 'Huisartsenpraktijk',
    typeLabelEn: 'General Practice',
    agbCode: '01234567',
    uraNumber: '12345678',
  },
  {
    id: 'org-3',
    name: 'Ziekenhuis Oost',
    type: 'hospital',
    typeLabel: 'Ziekenhuis',
    typeLabelEn: 'Hospital',
    agbCode: '98765432',
    uraNumber: '87654321',
  },
  {
    id: 'org-4',
    name: 'Verpleeghuis De Rusthoeve',
    type: 'care_home',
    typeLabel: 'Verpleeghuis',
    typeLabelEn: 'Care Home',
    agbCode: '11223344',
    uraNumber: '44332211',
  },
];

export function getOrganizationById(id: string): MockOrganization | undefined {
  return mockOrganizations.find((org) => org.id === id);
}
