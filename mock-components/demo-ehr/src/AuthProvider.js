import React, { createContext, useContext, useEffect, useState } from 'react';
import { authConfig } from './authConfig';
import { practitionerApi } from './api/practitionerApi';

const AuthContext = createContext();

export const useAuth = () => {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
};

export const AuthProvider = ({ children }) => {
  const [user, setUser] = useState(null);
  const [isLoading, setIsLoading] = useState(true);
  const [practitionerId, setPractitionerId] = useState(null);
  const ensuringPractitionerRef = React.useRef(false);

  // Function to ensure practitioner exists for the logged-in user
  const ensurePractitioner = async (user) => {
    // Prevent concurrent execution
    if (ensuringPractitionerRef.current) {
      console.log('Practitioner check already in progress, skipping...');
      return;
    }

    ensuringPractitionerRef.current = true;

    try {
      const userId = user?.sub || user?.dezi_nummer;
      const userName = user?.name || 'Unknown User';
      const userEmail = user?.email;

      if (!userId) {
        console.warn('No user ID found in profile');
        return;
      }

      // If we already have a practitioner ID, skip
      if (practitionerId) {
        console.log('Practitioner ID already set:', practitionerId);
        return;
      }

      // Search for existing practitioner by identifier
      console.log('Searching for Practitioner by identifier:', userId);
      let practitioner = await practitionerApi.searchByIdentifier(userId);

      if (practitioner) {
        // Practitioner found, store the ID in state
        console.log('Practitioner found:', practitioner.id);
        setPractitionerId(practitioner.id);
        return;
      }

      // No practitioner found, create new one
      console.log('No Practitioner found, creating new one for user:', userName);
      const newPractitioner = await practitionerApi.createPractitioner(userId, userName, userEmail);

      // Store the practitioner ID in state
      setPractitionerId(newPractitioner.id);
      console.log('Practitioner created and stored:', newPractitioner.id);
    } catch (err) {
      console.error('Error ensuring practitioner:', err);
    } finally {
      ensuringPractitionerRef.current = false;
    }
  };

  // Function to fetch user info from the auth server
  const fetchUserInfo = async () => {
    try {
      const response = await fetch(`${authConfig.baseUrl}/userinfo`, {
        credentials: 'include', // Include cookies
      });

      if (response.ok) {
        const userInfo = await response.json();
        setUser(userInfo);
        await ensurePractitioner(userInfo);
        return userInfo;
      } else if (response.status === 401) {
        // Not authenticated
        setUser(null);
        return null;
      } else {
        console.error('Failed to fetch user info:', response.statusText);
        return null;
      }
    } catch (err) {
      console.error('Error fetching user info:', err);
      return null;
    }
  };

  useEffect(() => {
    // Check if user is already authenticated on mount
    fetchUserInfo().finally(() => setIsLoading(false));
  }, []);

  const login = () => {
    // Redirect to the auth server's login endpoint
    const returnUrl = window.location.href;
    window.location.href = `${authConfig.baseUrl}/login?return_url=${encodeURIComponent(returnUrl)}`;
  };

  const logout = () => {
    setUser(null);
    setPractitionerId(null);
    window.location.href = `${authConfig.baseUrl}/logout`;
  };

  const value = {
    user,
    isLoading,
    isAuthenticated: !!user,
    practitionerId,
    login,
    logout,
  };

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
};

