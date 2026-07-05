import { Box, Card, CardContent, Typography } from '@mui/material';
import { PageHeader } from '../components/PageHeader';

type PlaceholderPageProps = {
  title: string;
};

export function PlaceholderPage({ title }: PlaceholderPageProps) {
  return (
    <>
      <PageHeader title={title} eyebrow="Planned module" />
      <Box sx={{ px: { xs: 2, md: 4 }, pb: 4 }}>
        <Card sx={{ maxWidth: 760 }}>
          <CardContent>
            <Typography color="text.secondary">
              This module is part of the MediaForge roadmap and will be implemented after the
              first manual scan, profile, and queue workflow is stable.
            </Typography>
          </CardContent>
        </Card>
      </Box>
    </>
  );
}

